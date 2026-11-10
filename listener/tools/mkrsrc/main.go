// mkrsrc writes rsrc_windows_amd64.syso so voiceloop.exe carries the vl icon and version info.
// Run from listener/: go run ./tools/mkrsrc
package main

import (
	"image"
	"log"
	"os"
	"regexp"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
	"voiceloop/listener/logo"
)

func main() {
	var imgs []image.Image
	for _, n := range []int{16, 20, 24, 32, 40, 48, 64, 128, 256} {
		imgs = append(imgs, logo.Render(n, logo.Teal))
	}
	icon, err := winres.NewIconFromImages(imgs)
	if err != nil {
		log.Fatal(err)
	}
	rs := winres.ResourceSet{}
	if err := rs.SetIcon(winres.Name("APPICON"), icon); err != nil {
		log.Fatal(err)
	}
	src, err := os.ReadFile("version.go")
	if err != nil {
		log.Fatal(err)
	}
	m := regexp.MustCompile(`const version = "([0-9.]+)"`).FindSubmatch(src)
	if m == nil {
		log.Fatal("version.go: no version constant")
	}
	v := version.Info{}
	v.SetFileVersion(string(m[1]))
	v.SetProductVersion(string(m[1]))
	v.Set(version.LangDefault, version.ProductName, "voiceloop")
	v.Set(version.LangDefault, version.FileDescription, "voiceloop: speaks Claude's replies")
	rs.SetVersionInfo(v)
	f, err := os.Create("rsrc_windows_amd64.syso")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := rs.WriteObject(f, winres.ArchAMD64); err != nil {
		log.Fatal(err)
	}
}
