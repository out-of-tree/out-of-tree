package distro

// ByRootFS is sorting by .RootFS lexicographically
type ByRootFS []KernelInfo

func (a ByRootFS) Len() int           { return len(a) }
func (a ByRootFS) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByRootFS) Less(i, j int) bool { return a[i].RootFS < a[j].RootFS }

// KernelInfo defines kernels.toml entries
type KernelInfo struct {
	Distro Distro

	// Must be *exactly* same as in `uname -r`
	KernelVersion string

	KernelRelease string

	// Build-time information
	KernelSource  string // module/exploit will be build on host
	ContainerName string

	// Runtime information
	KernelPath  string
	InitrdPath  string
	ModulesPath string

	RootFS string

	// Debug symbols
	VmlinuxPath string
}
