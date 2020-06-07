package cmd

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"transcoder/helper"
)


func prepareBuildEnv(path string) string {
	buildPath := filepath.Join(helper.GetWD(),"build",path)
	os.RemoveAll(buildPath)
	if err:=os.MkdirAll(buildPath,os.ModePerm);err!=nil && !os.IsExist(err) {
		panic(err)
	}
	return buildPath
}


func executeWithEnv(workingDir string, env []string,command string, arg ...string){
	log.Debugf("++ %s %s\n",command,strings.Join(arg," "))
	cmd := exec.Command(command, arg...)
	cmd.Dir = workingDir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Execute the command
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}


func execute(workingDir string,command string, arg ...string){
	executeWithEnv(workingDir,os.Environ(),command,arg...)
}

func getDependency() {
	execute(helper.GetWD(),"go","mod","download")
	execute(helper.GetWD(),"go","get","-u","github.com/rakyll/statik")
}

func getCapturingGroupsRegex(r *regexp.Regexp,parse string) map[string]string {
	capturedGroups := make(map[string]string)
	names := r.SubexpNames()
	res := r.FindAllStringSubmatch(parse,-1)
	for i, _ := range res[0] {
		if i != 0 {
			capturedGroups[names[i]]= res[0][i]
		}
	}
	return capturedGroups
}




func copyResources(buildPath string, sourcePath string, GOOS,GOARCH string)  error {
	archOSRegex := regexp.MustCompile(`(?P<GOOS>(darwin|linux|windows))-(?P<GOARCH>amd64)`)
	err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if archOSRegex.MatchString(path) {
			capturedGroups := getCapturingGroupsRegex(archOSRegex, path)
			if capturedGroups["GOOS"] != GOOS || capturedGroups["GOARCH"] != GOARCH {
				return nil
			}
		}
		relative, _ := filepath.Rel(sourcePath, path)
		buildPathRel := filepath.Join(buildPath, relative)
		if info.IsDir() {
			os.Mkdir(buildPathRel, os.ModePerm)
		} else {
			if _, err := helper.CopyFilePath(path, buildPathRel); err != nil {
				panic(err)
			}
		}

		return nil
	})
	return err
}

func statikEmbed(resources string, target string) {
	execute(helper.GetWD(),"statik",fmt.Sprintf("-src=%s",resources),fmt.Sprintf("-dest=%s",target),"-f")
}