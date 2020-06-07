package cmd

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
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
			buildServer(slice)
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
			buildWorker(slice,mode)
		}
	},
}

func buildServer(platforms []string) {
	log.Infof("Preparing Build Environment...")
	buildPath := prepareBuildEnv("server")
	log.Infof("Get Dependencies...")
	getDependency()
	log.Infof("Copy Resources...")
	copyServerResources(buildPath)
	log.Infof("Embeding resources...")
	statikEmbed(buildPath,filepath.Join(helper.GetWD(),"server"))
	for _,platform := range platforms {
		log.Infof("Building for %s",platform)
		pltSplit := strings.Split(platform,"-")
		GOOS:= pltSplit[0]
		GOARCH:= pltSplit[1]
		envs := os.Environ()
		envs = append(envs, fmt.Sprintf("GOARCH=%s",GOARCH))
		envs = append(envs, fmt.Sprintf("GOOS=%s",GOOS))
		envs = append(envs, "CGO_ENABLED=0")
		extension :=""
		if GOOS == "windows" {
			extension=".exe"
		}
		executeWithEnv(filepath.Join(helper.GetWD(),"server"),envs,"go","build","-o",fmt.Sprintf("%s/build/transcoderd-%s%s",helper.GetWD(),platform,extension))
	}
}

func copyServerResources(buildPath string) {
	serverResourcesPath := filepath.Join(helper.GetWD(),"server","resources")
	copyResources(buildPath, serverResourcesPath)
}



func buildWorker(platforms []string, buildMode string) {
	log.Infof("Preparing Build Environment...")
	buildPath := prepareBuildEnv("worker")
	log.Infof("Get Dependencies...")
	getDependency()
	log.Infof("Copy Resources...")
	copyWorkerResources(buildPath,buildMode)
	log.Infof("Embeding resources...")
	statikEmbed(buildPath,filepath.Join(helper.GetWD(),"worker"))
	for _,platform := range platforms {
		log.Infof("Building for %s",platform)
		pltSplit := strings.Split(platform,"-")
		GOOS:= pltSplit[0]
		GOARCH:= pltSplit[1]
		envs := os.Environ()
		envs = append(envs, fmt.Sprintf("GOARCH=%s",GOARCH))
		envs = append(envs, fmt.Sprintf("GOOS=%s",GOOS))
		envs = append(envs, "CGO_ENABLED=0")
		extension :=""
		extra:="-ldflags="
		if GOOS == "windows" {
			extension=".exe"
			if buildMode == "gui" {
				envs = append(envs,"GO111MODULE=on")
				extra="-ldflags=-H windowsgui"
			}
		}

		executeWithEnv(filepath.Join(helper.GetWD(),"worker"),envs,"go","build",extra,"-o",fmt.Sprintf("%s/build/transcoderw-%s-%s%s",helper.GetWD(),buildMode,platform,extension))
	}
}

func copyWorkerResources(buildPath string,buildMode string) {
	workerResourcesPath := filepath.Join(helper.GetWD(),"worker","resources")
	copyResources(buildPath, workerResourcesPath)
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