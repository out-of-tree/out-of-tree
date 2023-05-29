package debian

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"code.dumpstack.io/tools/out-of-tree/cache"
	"code.dumpstack.io/tools/out-of-tree/config"
	"code.dumpstack.io/tools/out-of-tree/container"
	"code.dumpstack.io/tools/out-of-tree/distro"
	"code.dumpstack.io/tools/out-of-tree/distro/debian/snapshot"
	"code.dumpstack.io/tools/out-of-tree/fs"
)

func init() {
	releases := []Release{
		Wheezy,
		Jessie,
		Stretch,
		Buster,
		Bullseye,
		Bookworm,
	}

	for _, release := range releases {
		distro.Register(Debian{release: release})
	}
}

type Debian struct {
	release Release
}

func (d Debian) Equal(dd distro.Distro) bool {
	if dd.ID != distro.Debian {
		return false
	}

	return ReleaseFromString(dd.Release) == d.release
}

func (d Debian) Distro() distro.Distro {
	return distro.Distro{distro.Debian, d.release.String()}
}

func (d Debian) Packages() (packages []string, err error) {
	c, err := container.New(d.Distro())
	if err != nil {
		return
	}

	err = c.Build(d.image(), d.envs(), d.runs())
	if err != nil {
		return
	}

	kernels, err := GetKernels()
	if err != nil {
		log.Error().Err(err).Msg("get kernels")
		return
	}

	for _, dk := range kernels {
		if d.release != dk.Release {
			continue
		}

		version := kver(dk.Version.Package)

		// filter out pre-release kernels
		switch dk.Release {
		case Wheezy:
			if version.LessThan(kver("3.2-rc0")) {
				continue
			}
		case Jessie:
			if version.LessThan(kver("3.16-rc0")) {
				continue
			}
		case Stretch:
			if version.LessThan(kver("4.9-rc0")) {
				continue
			}
		case Buster:
			if version.LessThan(kver("4.19-rc0")) {
				continue
			}
		case Bullseye:
			if version.LessThan(kver("5.10-rc0")) {
				continue
			}
		case Bookworm:
			if version.LessThan(kver("6.1-rc0")) {
				continue
			}
		}

		p := dk.Image.Deb.Name[:len(dk.Image.Deb.Name)-4] // w/o .deb
		packages = append(packages, p)
	}

	return
}

type Release int

const (
	None Release = iota
	Buzz
	Hamm
	Woody
	Etch
	Lenny
	Squeeze
	Wheezy
	Jessie
	Stretch
	Buster
	Bullseye
	Bookworm
)

var ReleaseStrings = [...]string{
	"",
	"buzz",
	"hamm",
	"woody",
	"etch",
	"lenny",
	"squeeze",
	"wheezy",
	"jessie",
	"stretch",
	"buster",
	"bullseye",
	"bookworm",
}

func (cn Release) Name() string {
	return ReleaseStrings[cn]
}

func (cn Release) String() string {
	return fmt.Sprintf("%d", cn)
}

func ReleaseFromString(s string) (r Release) {
	switch strings.ToLower(s) {
	case "1", "buzz":
		r = Buzz
	case "2", "hamm":
		r = Hamm
	case "3", "woody":
		r = Woody
	case "4", "etch":
		r = Etch
	case "5", "lenny":
		r = Lenny
	case "6", "squeeze":
		r = Squeeze
	case "7", "wheezy":
		r = Wheezy
	case "8", "jessie":
		r = Jessie
	case "9", "stretch":
		r = Stretch
	case "10", "buster":
		r = Buster
	case "11", "bullseye":
		r = Bullseye
	case "12", "bookworm":
		r = Bookworm
	default:
		r = None
	}
	return
}

func (d Debian) envs() (envs []string) {
	envs = append(envs, "DEBIAN_FRONTEND=noninteractive")
	return
}

func (d Debian) image() (image string) {
	image += "debian:"

	switch d.release {
	case Wheezy:
		image += "wheezy-20190228"
	case Jessie:
		image += "jessie-20210326"
	case Stretch:
		image += "stretch-20220622"
	default:
		image += d.release.Name()
	}

	return
}

func repositories(release Release) (repos []string) {
	var snapshot string

	switch release {
	// Latest snapshots that include release
	case Wheezy:
		// doesn't include snapshot repos in /etc/apt/source.list
		snapshot = "20190321T212815Z"
	case Jessie:
		snapshot = "20230322T152120Z"
	case Stretch:
		snapshot = "20230423T032533Z"
	default:
		return
	}

	repo := func(archive, s string) {
		format := "deb [check-valid-until=no trusted=yes] " +
			"http://snapshot.debian.org/archive/%s/%s " +
			"%s%s main"
		r := fmt.Sprintf(format, archive, snapshot, release.Name(), s)
		repos = append(repos, r)
	}

	repo("debian", "")
	repo("debian", "-updates")
	if release <= 7 {
		repo("debian", "-backports")
	}
	repo("debian-security", "/updates")

	return
}

func (d Debian) runs() (commands []string) {
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	repos := repositories(d.release)

	if len(repos) != 0 {
		cmdf("rm /etc/apt/sources.list")
		for _, repo := range repos {
			cmdf("echo '%s' >> /etc/apt/sources.list", repo)
		}
	} else {
		cmdf("apt-get update || sed -i " +
			"-e '/snapshot/!d' " +
			"-e 's/# deb/deb [check-valid-until=no trusted=yes]/' " +
			"/etc/apt/sources.list")
	}

	cmdf("apt-get update || apt-get update || apt-get update")

	pkglist := []string{
		"wget", "build-essential", "libelf-dev", "git",
		"kmod", "linux-base", "libssl-dev",
		"firmware-linux-free",
	}

	gccs := "'^(gcc-[0-9].[0-9]|gcc-[0-9]|gcc-[1-9][0-9])$'"
	pkglist = append(pkglist, gccs)

	if d.release >= 8 {
		pkglist = append(pkglist, "initramfs-tools")
	} else {
		// by default Debian backports repositories have a lower
		// priority than stable, so we should specify it manually
		cmdf("apt-get -y install -t %s-backports "+
			"initramfs-tools", d.release.Name())
	}

	if d.release < 9 {
		pkglist = append(pkglist, "module-init-tools")
	}

	var packages string
	for _, pkg := range pkglist {
		packages += fmt.Sprintf("%s ", pkg)
	}

	cmdf("timeout 5m apt-get install -y %s "+
		"|| timeout 10m apt-get install -y %s "+
		"|| apt-get install -y %s", packages, packages, packages)

	cmdf("mkdir -p /lib/modules")

	return
}

func (d Debian) Kernels() (kernels []distro.KernelInfo, err error) {
	c, err := container.New(d.Distro())
	if err != nil {
		return
	}

	if !c.Exist() {
		return
	}

	cpath := config.Dir("volumes", c.Name())
	rootfs := config.File("images", c.Name()+".img")

	files, err := os.ReadDir(cpath)
	if err != nil {
		return
	}

	for _, file := range files {
		if !strings.Contains(file.Name(), "linux-image") {
			continue
		}

		pkgname := file.Name()

		kpkgdir := filepath.Join(cpath, pkgname)

		bootdir := filepath.Join(kpkgdir, "boot")

		vmlinuz, err := fs.FindBySubstring(bootdir, "vmlinuz")
		if err != nil {
			log.Warn().Msgf("cannot find vmlinuz for %s", pkgname)
			continue
		}

		initrd, err := fs.FindBySubstring(bootdir, "initrd")
		if err != nil {
			log.Warn().Msgf("cannot find initrd for %s", pkgname)
			continue
		}

		modulesdir := filepath.Join(kpkgdir, "lib/modules")

		modules, err := fs.FindBySubstring(modulesdir, "")
		if err != nil {
			log.Warn().Msgf("cannot find modules for %s", pkgname)
			continue
		}

		log.Debug().Msgf("%s %s %s", vmlinuz, initrd, modules)

		release := strings.Replace(pkgname, "linux-image-", "", -1)

		ki := distro.KernelInfo{
			Distro:        d.Distro(),
			KernelVersion: path.Base(modules),
			KernelRelease: release,
			ContainerName: c.Name(),

			KernelPath:  vmlinuz,
			InitrdPath:  initrd,
			ModulesPath: modules,

			RootFS: rootfs,

			Package: pkgname,
		}

		kernels = append(kernels, ki)
	}

	return
}

func (d Debian) volumes(pkgname string) (volumes []container.Volume) {
	c, err := container.New(d.Distro())
	if err != nil {
		return
	}

	pkgdir := filepath.Join("volumes", c.Name(), pkgname)

	volumes = append(volumes, container.Volume{
		Src:  config.Dir(pkgdir, "/lib/modules"),
		Dest: "/lib/modules",
	})

	volumes = append(volumes, container.Volume{
		Src:  config.Dir(pkgdir, "/usr/src"),
		Dest: "/usr/src",
	})

	volumes = append(volumes, container.Volume{
		Src:  config.Dir(pkgdir, "/boot"),
		Dest: "/boot",
	})

	return
}

func (d Debian) Install(pkgname string, headers bool) (err error) {
	defer func() {
		if err != nil {
			d.cleanup(pkgname)
		}
	}()

	dk, err := getCachedKernel(pkgname + ".deb")
	if err != nil {
		return
	}

	var pkgs []snapshot.Package
	if headers {
		pkgs = dk.Packages()
	} else {
		pkgs = []snapshot.Package{dk.Image}
	}

	var commands []string
	cmdf := func(f string, s ...interface{}) {
		commands = append(commands, fmt.Sprintf(f, s...))
	}

	for _, pkg := range pkgs {
		found, newurl := cache.PackageURL(
			distro.Debian,
			pkg.Deb.URL,
		)
		if found {
			log.Debug().Msgf("cached deb found %s", newurl)
			pkg.Deb.URL = newurl
		}

		// TODO use faketime on old releases?
		pkg.Deb.URL = strings.Replace(pkg.Deb.URL, "https", "http", -1)

		cmdf("wget --no-verbose " +
			"--timeout=10 --waitretry=1 --tries=10 " +
			"--no-check-certificate " + pkg.Deb.URL)
	}

	// prepare local repository
	cmdf("mkdir debs && mv *.deb debs/")
	cmdf("dpkg-scanpackages debs /dev/null | gzip > debs/Packages.gz")
	cmdf(`echo "deb [trusted=yes] file:$(pwd) debs/" >> /etc/apt/sources.list.d/local.list`)
	cmdf("apt-get update -o Dir::Etc::sourcelist='sources.list.d/local.list' -o Dir::Etc::sourceparts='-' -o APT::Get::List-Cleanup='0'")

	// make sure apt-get will not download the repo version
	cmdf("echo 'Package: *' >> /etc/apt/preferences.d/pin")
	cmdf(`echo 'Pin: origin "*.debian.org"' >> /etc/apt/preferences.d/pin`)
	cmdf("echo 'Pin-Priority: 100' >> /etc/apt/preferences.d/pin")

	// cut package names and install
	cmdf("ls debs | grep deb | cut -d '_' -f 1 | " +
		"xargs apt-get -y --force-yes install")

	// for debug
	cmdf("ls debs | grep deb | cut -d '_' -f 1 | xargs apt-cache policy")

	c, err := container.New(d.Distro())
	if err != nil {
		return
	}

	c.Volumes = d.volumes(pkgname)
	for i := range c.Volumes {
		c.Volumes[i].Dest = "/target" + c.Volumes[i].Dest
	}

	cmdf("cp -r /boot /target/")
	cmdf("cp -r /lib/modules /target/lib/")
	cmdf("cp -rL /usr/src /target/usr/")

	_, err = c.Run("", commands)
	if err != nil {
		return
	}

	return
}

func (d Debian) cleanup(pkgname string) {
	c, err := container.New(d.Distro())
	if err != nil {
		return
	}

	pkgdir := config.Dir(filepath.Join("volumes", c.Name(), pkgname))

	log.Debug().Msgf("cleanup %s", pkgdir)

	err = os.RemoveAll(pkgdir)
	if err != nil {
		log.Warn().Err(err).Msg("cleanup")
	}
}
