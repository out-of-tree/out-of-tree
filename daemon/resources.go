package daemon

import (
	"errors"
	"runtime"
	"sync"
	"syscall"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/api"
)

type Resources struct {
	initialized bool

	CPU *CPUResource
	RAM *RAMResources
}

func NewResources() (r *Resources) {
	r = &Resources{}
	r.CPU = NewCPUResources()
	r.RAM = NewRAMResources()
	r.initialized = true
	return
}

func (r *Resources) Allocate(job api.Job) (err error) {
	if !r.initialized {
		err = errors.New("resources not initialized")
		return
	}

	if job.Artifact.Qemu.Cpus == 0 {
		err = errors.New("no cpus requested")
		return
	}

	if job.Artifact.Qemu.Memory == 0 {
		err = errors.New("no memory requested")
		return
	}

	origRam := r.RAM.GetSpent()
	origCPU := r.CPU.GetSpent()

	err = r.CPU.Allocate(job.Artifact.Qemu.Cpus)
	if err != nil {
		return
	}

	err = r.RAM.Allocate(job.Artifact.Qemu.Memory)
	if err != nil {
		r.CPU.Release(job.Artifact.Qemu.Cpus)
		return
	}

	log.Debug().Msgf("allocated %d cpus, %d MB ram",
		r.CPU.GetSpent()-origCPU,
		r.RAM.GetSpent()-origRam)

	return
}

func (r *Resources) Release(job api.Job) {
	if !r.initialized {
		log.Error().Msg("resources not initialized")
		return
	}

	r.CPU.Release(job.Artifact.Qemu.Cpus)
	r.RAM.Release(job.Artifact.Qemu.Memory)

	log.Debug().Msgf("released %d cpus, %d MB ram",
		job.Artifact.Qemu.Cpus,
		job.Artifact.Qemu.Memory)
}

type CPUResource struct {
	num        int
	overcommit float64

	mu    *sync.Mutex
	spent int
}

const (
	Allocation = iota
	Release
)

func NewCPUResources() (cpur *CPUResource) {
	cpur = &CPUResource{}
	cpur.mu = &sync.Mutex{}
	cpur.num = runtime.NumCPU()
	cpur.overcommit = 1
	log.Debug().Msgf("total cpus: %d", cpur.num)
	return
}

func (cpur *CPUResource) SetOvercommit(oc float64) {
	log.Info().Int("cpus", cpur.num).
		Int("result", int(float64(cpur.num)*oc)).
		Msgf("%.02f", oc)
	cpur.overcommit = oc
}

func (cpur *CPUResource) GetSpent() int {
	cpur.mu.Lock()
	defer cpur.mu.Unlock()
	return cpur.spent
}

var ErrNotEnoughCpu = errors.New("not enough cpu")

func (cpur *CPUResource) Allocate(cpu int) (err error) {
	cpur.mu.Lock()
	defer cpur.mu.Unlock()

	if cpur.spent+cpu > int(float64(cpur.num)*cpur.overcommit) {
		err = ErrNotEnoughCpu
		return
	}

	cpur.spent += cpu
	return
}

func (cpur *CPUResource) Release(cpu int) (err error) {
	cpur.mu.Lock()
	defer cpur.mu.Unlock()

	if cpur.spent < cpu {
		err = ErrFreeingMoreThanAllocated
		return
	}

	cpur.spent -= cpu
	return
}

type RAMResources struct {
	mb         int
	overcommit float64

	mu    *sync.Mutex
	spent int
}

func NewRAMResources() (ramr *RAMResources) {
	ramr = &RAMResources{}
	ramr.mu = &sync.Mutex{}
	ramr.overcommit = 1

	var info syscall.Sysinfo_t
	syscall.Sysinfo(&info)
	ramr.mb = int(info.Totalram / 1024 / 1024)
	log.Debug().Msgf("total ram: %d MB", ramr.mb)
	return
}

func (ramr *RAMResources) SetOvercommit(oc float64) {
	log.Info().Int("ram", ramr.mb).
		Int("result", int(float64(ramr.mb)*oc)).
		Msgf("%.02f", oc)
	ramr.overcommit = oc
}

func (ramr RAMResources) GetSpent() int {
	ramr.mu.Lock()
	defer ramr.mu.Unlock()
	return ramr.spent
}

var ErrNotEnoughRam = errors.New("not enough ram")

func (ramr *RAMResources) Allocate(mb int) (err error) {
	ramr.mu.Lock()
	defer ramr.mu.Unlock()

	ocmem := int(float64(ramr.mb) * ramr.overcommit)

	if mb > ocmem-ramr.spent {
		err = ErrNotEnoughRam
		return
	}

	ramr.spent += mb
	return
}

var ErrFreeingMoreThanAllocated = errors.New("freeing more than allocated")

func (ramr *RAMResources) Release(mb int) (err error) {
	ramr.mu.Lock()
	defer ramr.mu.Unlock()

	if ramr.spent < mb {
		err = ErrFreeingMoreThanAllocated
		return
	}

	ramr.spent -= mb
	return
}
