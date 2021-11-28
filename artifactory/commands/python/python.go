package python

import (
	"errors"
	"fmt"
	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/python/dependencies"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
)

type PythonCommand struct {
	serverDetails          *config.ServerDetails
	executable             string
	commandName            string
	args                   []string
	repository             string
	buildConfiguration     *utils.BuildConfiguration
	shouldCollectBuildInfo bool
}

func (pc *PythonCommand) SetServerDetails(serverDetails *config.ServerDetails) *PythonCommand {
	pc.serverDetails = serverDetails
	return pc
}

func (pc *PythonCommand) SetRepo(repo string) *PythonCommand {
	pc.repository = repo
	return pc
}

func (pc *PythonCommand) SetArgs(arguments []string) *PythonCommand {
	pc.args = arguments
	return pc
}

func (pc *PythonCommand) SetCommandName(commandName string) *PythonCommand {
	pc.commandName = commandName
	return pc
}

func (pc *PythonCommand) collectBuildInfo(cacheDirPath string, allDependencies map[string]*buildinfo.Dependency) error {
	if err := pc.determineModuleName(); err != nil {
		return err
	}
	// Populate dependencies information - checksums and file-name.
	servicesManager, err := utils.CreateServiceManager(pc.serverDetails, -1, false)
	if err != nil {
		return err
	}
	err = dependencies.UpdateDepsChecksumInfo(allDependencies, cacheDirPath, servicesManager, pc.repository)
	if err != nil {
		return err
	}
	err = dependencies.UpdateDependenciesCache(allDependencies, cacheDirPath)
	if err != nil {
		return err
	}
	return pc.saveBuildInfo(allDependencies)
}

func (pc *PythonCommand) saveBuildInfo(allDependencies map[string]*buildinfo.Dependency) error {
	buildInfo := &buildinfo.BuildInfo{}
	var modules []buildinfo.Module
	var projectDependencies []buildinfo.Dependency

	for _, dep := range allDependencies {
		projectDependencies = append(projectDependencies, *dep)
	}

	// Save build-info.
	module := buildinfo.Module{Id: pc.buildConfiguration.Module, Type: buildinfo.Python, Dependencies: projectDependencies}
	modules = append(modules, module)

	buildInfo.Modules = modules
	return utils.SaveBuildInfo(pc.buildConfiguration.BuildName, pc.buildConfiguration.BuildNumber, pc.buildConfiguration.Project, buildInfo)
}

func (pc *PythonCommand) determineModuleName() error {
	pythonExecutablePath, err := getExecutablePath("python")
	if err != nil {
		return err
	}
	// If module-name was set by the command, don't change it.
	if pc.buildConfiguration.Module != "" {
		return nil
	}

	// Get package-name.
	moduleName, err := getPackageName(pythonExecutablePath)
	if err != nil {
		return err
	}

	// If the package name is unknown, set the module name to be the build name.
	if moduleName == "" {
		moduleName = pc.buildConfiguration.BuildName
	}

	pc.buildConfiguration.Module = moduleName
	return nil
}

func (pc *PythonCommand) prepareBuildPrerequisites() (err error) {
	log.Debug("Preparing build prerequisites...")
	pc.args, pc.buildConfiguration, err = utils.ExtractBuildDetailsFromArgs(pc.args)
	if err != nil {
		return
	}

	// Prepare build-info.
	if pc.buildConfiguration.BuildName != "" && pc.buildConfiguration.BuildNumber != "" {
		pc.shouldCollectBuildInfo = true
		if err = utils.SaveBuildGeneralDetails(pc.buildConfiguration.BuildName, pc.buildConfiguration.BuildNumber, pc.buildConfiguration.Project); err != nil {
			return
		}
	}
	return
}

func getExecutablePath(executableName string) (executablePath string, err error) {
	executablePath, err = exec.LookPath(executableName)
	if err != nil {
		return
	}
	if executablePath == "" {
		return "", errorutils.CheckError(errors.New("Could not find the" + executableName + " executable in the system PATH"))
	}

	return executablePath, nil
}

func getPackageName(pythonExecutablePath string) (string, error) {
	filePath, err := getSetupPyFilePath()
	if err != nil || filePath == "" {
		// Error was returned or setup.py does not exist in directory.
		return "", err
	}

	// Extract package name from setup.py.
	packageName, err := ExtractPackageNameFromSetupPy(filePath, pythonExecutablePath)
	if err != nil {
		return "", errors.New("Failed determining module-name from 'setup.py' file: " + err.Error())
	}
	return packageName, err
}

// Look for 'setup.py' file in current work dir.
// If found, return its absolute path.
func getSetupPyFilePath() (string, error) {
	wd, err := os.Getwd()
	if errorutils.CheckError(err) != nil {
		return "", err
	}

	filePath := filepath.Join(wd, "setup.py")
	// Check if setup.py exists.
	validPath, err := fileutils.IsFileExists(filePath, false)
	if err != nil {
		return "", err
	}
	if !validPath {
		log.Debug("Could not find setup.py file in current directory:", wd)
		return "", nil
	}

	return filePath, nil
}

func (pc *PythonCommand) cleanBuildInfoDir() {
	if err := utils.RemoveBuildDir(pc.buildConfiguration.BuildName, pc.buildConfiguration.BuildNumber, pc.buildConfiguration.Project); err != nil {
		log.Error(fmt.Sprintf("Failed cleaning build-info directory: %s", err.Error()))
	}
}

func (pc *PythonCommand) setPypiRepoUrlWithCredentials(serverDetails *config.ServerDetails, repository string, projectType utils.ProjectType) error {
	rtUrl, err := url.Parse(serverDetails.GetArtifactoryUrl())
	if err != nil {
		return errorutils.CheckError(err)
	}

	username := serverDetails.GetUser()
	password := serverDetails.GetPassword()

	// Get credentials from access-token if exists.
	if serverDetails.GetAccessToken() != "" {
		username, err = auth.ExtractUsernameFromAccessToken(serverDetails.GetAccessToken())
		if err != nil {
			return err
		}
		password = serverDetails.GetAccessToken()
	}

	if username != "" && password != "" {
		rtUrl.User = url.UserPassword(username, password)
	}
	rtUrl.Path += "api/pypi/" + repository + "/simple"

	if projectType == utils.Pip {
		pc.args = append(pc.args, "-i")
	} else if projectType == utils.Pipenv {
		pc.args = append(pc.args, "--pypi-mirror")
	}
	pc.args = append(pc.args, rtUrl.String())
	return nil
}

func (pc *PythonCommand) GetCmd() *exec.Cmd {
	var cmd []string
	cmd = append(cmd, pc.executable)
	cmd = append(cmd, pc.commandName)
	cmd = append(cmd, pc.args...)
	return exec.Command(cmd[0], cmd[1:]...)
}

func (pc *PythonCommand) GetEnv() map[string]string {
	return map[string]string{}
}

func (pc *PythonCommand) GetStdWriter() io.WriteCloser {
	return nil
}

func (pc *PythonCommand) GetErrWriter() io.WriteCloser {
	return nil
}
