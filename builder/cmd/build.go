package cmd

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"transcoder/builder/mac"
	"transcoder/helper"
)
var allPlatforms = []string{"windows-amd64","linux-amd64","darwin-amd64"}
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "b",
	Long:  `Build server or worker`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("build called")
	},
}

var buildServerCmd = &cobra.Command{
	Use:   "server",
	Short: "s",
	Long:  `server build`,

	Run: func(cmd *cobra.Command, args []string) {
		slice, _ := cmd.Flags().GetStringSlice("platform")
		if slice[0]=="all" {
			buildServer(allPlatforms)
			return
		}else{
			buildServer(cleanPlatforms(slice))
		}
	},
}

var buildWorkerCmd = &cobra.Command{
	Use:   "worker",
	Short: "w",
	Long:  `worker build`,
	Run: func(cmd *cobra.Command, args []string) {
		mode, _ := cmd.Flags().GetString("mode")
		slice, _ := cmd.Flags().GetStringSlice("platform")
		if slice[0]=="all" {
			buildWorker(allPlatforms,mode)
			return
		}else{
			buildWorker(cleanPlatforms(slice),mode)
		}
	},
}

func cleanPlatforms(platforms []string) []string{
	for i,platform := range platforms {
		if platform == "macos" {
			platforms[i]="darwin"
		}
		if platform == "ubuntu" {
			platforms[i]="linux"
		}
	}
}

func buildServer(platforms []string) {
	log.Infof("Get Dependencies...")
	getDependency()

	for _,platform := range platforms {
		log.Infof("====== %s ======",strings.ToUpper(platform))
		pltSplit := strings.Split(platform,"-")
		GOOS:= pltSplit[0]
		GOARCH:= pltSplit[1]
		log.Infof("[%s] Preparing Build Environment...",platform)
		buildPath := prepareBuildEnv("server")
		log.Infof("[%s] Copy Resources...",platform)
		copyServerResources(buildPath,GOOS,GOARCH)
		log.Infof("[%s] Embedding resources...",platform)
		statikEmbed(buildPath,filepath.Join(helper.GetWD(),"server"))

		envs := os.Environ()
		envs = append(envs, fmt.Sprintf("GOARCH=%s",GOARCH))
		envs = append(envs, fmt.Sprintf("GOOS=%s",GOOS))
		envs = append(envs, "CGO_ENABLED=0")
		extension :=""
		if GOOS == "windows" {
			extension=".exe"
		}
		log.Infof("[%s] Building executable...",platform)
		executeWithEnv(filepath.Join(helper.GetWD(),"server"),envs,"go","build","-o",fmt.Sprintf("%s/build/transcoderd-%s%s",helper.GetWD(),platform,extension))
	}
}

func copyServerResources(buildPath string,GOOS,GOARCH string) {
	serverResourcesPath := filepath.Join(helper.GetWD(),"server","resources")
	copyResources(buildPath, serverResourcesPath,GOOS,GOARCH)
}



func buildWorker(platforms []string, buildMode string) {
	log.Infof("Get Dependencies...")
	getDependency()

	for _,platform := range platforms {
		log.Infof("====== %s ======",strings.ToUpper(platform))
		pltSplit := strings.Split(platform,"-")
		GOOS:= pltSplit[0]
		GOARCH:= pltSplit[1]

		log.Infof("[%s] Preparing Build Environment...",platform)
		buildPath := prepareBuildEnv("worker")

		log.Infof("[%s] Copy Resources...",platform)
		copyWorkerResources(buildPath,buildMode,GOOS,GOARCH)
		log.Infof("[%s] Embedding resources...",platform)
		statikEmbed(buildPath,filepath.Join(helper.GetWD(),"worker"))
		envs := os.Environ()
		envs = append(envs, fmt.Sprintf("GOARCH=%s",GOARCH))
		envs = append(envs, fmt.Sprintf("GOOS=%s",GOOS))
		extension :=""
		extra:="-ldflags="
		if GOOS == "windows" {
			envs = append(envs, "CGO_ENABLED=0")
			extension=".exe"
			if buildMode == "gui" {
				extra="-ldflags=-H windowsgui"
			}
		} else if GOOS == "linux" {
			envs = append(envs, "CGO_ENABLED=1")
		} else if GOOS == "darwin" {
			envs = append(envs, "CGO_ENABLED=1")
			envs = append(envs,"GO111MODULE=on")
		}
		log.Infof("[%s] Building executable...",platform)
		fileName := fmt.Sprintf("transcoderw-%s-%s%s",buildMode,platform,extension)
		outputBinpath := fmt.Sprintf("%s/build/%s",helper.GetWD(),fileName)
		executeWithEnv(filepath.Join(helper.GetWD(),"worker"),envs,"go","build",extra,"-o",outputBinpath)
		if GOOS == "darwin" && buildMode=="gui" {
			//TODO Builder Version??
			sysTrayIco:=filepath.Join(helper.GetWD(),"worker","resources","systray.png")
			mac.Package(fileName,"","1.0","",sysTrayIco,outputBinpath)
		}
	}
}

func copyWorkerResources(buildPath string,buildMode string,GOOS,GOARCH string) {
	workerResourcesPath := filepath.Join(helper.GetWD(),"worker","resources")
	copyResources(buildPath, workerResourcesPath,GOOS,GOARCH)
	if buildMode == "gui" {
		sysF, err := os.OpenFile(filepath.Join(buildPath,"systray.enabled"),os.O_TRUNC|os.O_CREATE|os.O_RDWR, os.ModePerm)
		if err!=nil {
			panic(err)
		}
		defer sysF.Close()
		sysF.WriteString("1")
		sysF.Sync()
	}
}



func init() {
	buildCmd.AddCommand(buildServerCmd,buildWorkerCmd)
	RootCmd.AddCommand(buildCmd)

	buildCmd.PersistentFlags().StringSliceP("platform","p", []string{"all"}, "select all platforms that you want to build (all,windows-amd64,linux-amd64,darwin-amd64,linux-arm,...)")
	buildCmd.PersistentFlags().StringP("mode","m", "gui", "Build for gui or console")
}