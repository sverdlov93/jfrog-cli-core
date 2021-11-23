package mvnutils

import (
	"bytes"
	"fmt"
	"github.com/jfrog/jfrog-client-go/utils/version"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/spf13/viper"
)

const (
	mavenExtractorDependencyVersion = "2.30.2"
	classworldsConfFileName         = "classworlds.conf"
	mavenHome                       = "M2_HOME"
	minSupportedMvnVersion          = "3.1.0"
)

func RunMvn(configPath, deployableArtifactsFile string, buildConf *utils.BuildConfiguration, goals []string, threads int, insecureTls, disableDeploy bool) error {
	log.Info("Running Mvn...")
	mvnHome, err := getMavenHomeAndValidateVersion()
	if err != nil {
		return err
	}

	var dependenciesPath string
	dependenciesPath, err = downloadDependencies()
	if err != nil {
		return err
	}

	mvnRunConfig, err := createMvnRunConfig(dependenciesPath, configPath, deployableArtifactsFile, mvnHome, buildConf, goals, threads, insecureTls, disableDeploy)
	if err != nil {
		return err
	}

	defer os.Remove(mvnRunConfig.buildInfoProperties)
	return mvnRunConfig.runCmd()
}

func getMavenHomeAndValidateVersion() (string, error) {
	log.Debug("Checking prerequisites.")
	mavenHome := os.Getenv(mavenHome)
	mvnVersion := ""

	output, err := runMvnVersionCommand(mavenHome)
	if err != nil {
		return "", err
	}
	// Finding the relevant "Maven home" line in command response.
	for _, line := range output {
		if mavenHome == "" && strings.HasPrefix(line, "Maven home:") {
			// The M2_HOME environment variable is not defined.
			// Since Maven installation can be located in different locations,
			// Depending on the installation type and the OS (for example: For Mac with brew install: /usr/local/Cellar/maven/{version}/libexec or Ubuntu with debian: /usr/share/maven),
			mavenHome, err = parseMvnHome(line)
			if err != nil {
				return "", err
			}
		}
		if strings.HasPrefix(line, "Apache Maven") {
			mvnVersion = strings.Split(line, " ")[2]
		}
	}

	if mavenHome == "" {
		return "", errorutils.CheckErrorf("Could not find the location of the maven home directory, by running 'mvn --version' command. The command output is:\n" + strings.Join(output, " ") + "\nYou also have the option of setting the M2_HOME environment variable value to the maven installation directory, which is the directory which includes the bin and lib directories.")
	}

	if mvnVersion == "" {
		return "", errorutils.CheckErrorf("Could not parse mvn version by running 'mvn --version' command. The command output is:\n" + strings.Join(output, " "))
	}

	err = validateMinimumVersion(mvnVersion)
	if err != nil {
		return "", err
	}
	log.Debug("Maven home location: ", mavenHome)
	log.Debug("Maven version: ", mvnVersion)
	return mavenHome, nil
}

func runMvnVersionCommand(mavenHome string) ([]string, error) {
	mvnPath := ""
	var err error
	if mavenHome != "" {
		mvnPath = filepath.Join(mavenHome, "bin", "mvn")
	} else {
		mvnPath, err = exec.LookPath("mvn")
		if err != nil || mvnPath == "" {
			return nil, errorutils.CheckErrorf(err.Error() + "Hint: The mvn command may not be included in the PATH. Either add it to the path, or set the M2_HOME environment variable value to the maven installation directory, which is the directory which includes the bin and lib directories.")
		}
	}
	cmd := exec.Command(mvnPath, "--version")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err = errorutils.CheckError(cmd.Run())
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(stdout.String()), "\n"), nil
}

func parseMvnHome(line string) (string, error) {
	mavenHome := strings.Split(line, " ")[2]
	if coreutils.IsWindows() {
		mavenHome = strings.TrimSuffix(mavenHome, "\r")
	}
	mavenHome, err := filepath.Abs(mavenHome)
	if err != nil {
		return "", err
	}
	return mavenHome, nil
}

func validateMinimumVersion(mvnVersion string) error {
	ver := version.NewVersion(mvnVersion)
	if ver.Compare(minSupportedMvnVersion) > 0 {
		return errorutils.CheckErrorf(
			"JFrog CLI mvn commands requires maven client version "+minSupportedMvnVersion+" or higher. The Current version is: %s", mvnVersion)
	}
	return nil
}

func downloadDependencies() (string, error) {
	dependenciesPath, err := config.GetJfrogDependenciesPath()
	if err != nil {
		return "", err
	}
	dependenciesPath = filepath.Join(dependenciesPath, "maven", mavenExtractorDependencyVersion)

	filename := fmt.Sprintf("build-info-extractor-maven3-%s-uber.jar", mavenExtractorDependencyVersion)
	filePath := fmt.Sprintf("org/jfrog/buildinfo/build-info-extractor-maven3/%s", mavenExtractorDependencyVersion)
	downloadPath := path.Join(filePath, filename)

	err = utils.DownloadExtractorIfNeeded(downloadPath, filepath.Join(dependenciesPath, filename))
	if err != nil {
		return "", err
	}

	err = createClassworldsConfig(dependenciesPath)
	return dependenciesPath, err
}

func createClassworldsConfig(dependenciesPath string) error {
	classworldsPath := filepath.Join(dependenciesPath, classworldsConfFileName)

	if fileutils.IsPathExists(classworldsPath, false) {
		return nil
	}
	return errorutils.CheckError(ioutil.WriteFile(classworldsPath, []byte(utils.ClassworldsConf), 0644))
}

func createMvnRunConfig(dependenciesPath, configPath, deployableArtifactsFile, mavenHome string, buildConf *utils.BuildConfiguration, goals []string, threads int, insecureTls, disableDeploy bool) (*mvnRunConfig, error) {
	var err error
	var javaExecPath string

	javaHome := os.Getenv("JAVA_HOME")
	if javaHome != "" {
		javaExecPath = filepath.Join(javaHome, "bin", "java")
	} else {
		javaExecPath, err = exec.LookPath("java")
		if err != nil {
			return nil, errorutils.CheckError(err)
		}
	}

	plexusClassworlds, err := filepath.Glob(filepath.Join(mavenHome, "boot", "plexus-classworlds*.jar"))
	if err != nil {
		return nil, errorutils.CheckError(err)
	}

	mavenOpts := os.Getenv("MAVEN_OPTS")

	if len(plexusClassworlds) < 1 {
		return nil, errorutils.CheckErrorf("couldn't find plexus-classworlds-x.x.x.jar in Maven installation path, please check M2_HOME environment variable")
	}

	var currentWorkdir string
	currentWorkdir, err = os.Getwd()
	if err != nil {
		return nil, errorutils.CheckError(err)
	}

	var vConfig *viper.Viper
	if configPath == "" {
		vConfig = viper.New()
		vConfig.SetConfigType(string(utils.YAML))
		vConfig.Set("type", utils.Maven.String())
	} else {
		vConfig, err = utils.ReadConfigFile(configPath, utils.YAML)
		if err != nil {
			return nil, err
		}
	}

	if len(buildConf.BuildName) > 0 && len(buildConf.BuildNumber) > 0 {
		vConfig.Set(utils.BuildName, buildConf.BuildName)
		vConfig.Set(utils.BuildNumber, buildConf.BuildNumber)
		vConfig.Set(utils.BuildProject, buildConf.Project)
		err = utils.SaveBuildGeneralDetails(buildConf.BuildName, buildConf.BuildNumber, buildConf.Project)
		if err != nil {
			return nil, err
		}
	}
	vConfig.Set(utils.InsecureTls, insecureTls)

	if threads > 0 {
		vConfig.Set(utils.ForkCount, threads)
	}

	if !vConfig.IsSet("deployer") {
		setEmptyDeployer(vConfig)
	}

	if disableDeploy {
		setDeployFalse(vConfig)
	}

	buildInfoProperties, err := utils.CreateBuildInfoPropertiesFile(buildConf.BuildName, buildConf.BuildNumber, buildConf.Project, deployableArtifactsFile, vConfig, utils.Maven)
	if err != nil {
		return nil, err
	}

	return &mvnRunConfig{
		java:                         javaExecPath,
		pluginDependencies:           dependenciesPath,
		plexusClassworlds:            plexusClassworlds[0],
		cleassworldsConfig:           filepath.Join(dependenciesPath, classworldsConfFileName),
		mavenHome:                    mavenHome,
		workspace:                    currentWorkdir,
		goals:                        goals,
		buildInfoProperties:          buildInfoProperties,
		artifactoryResolutionEnabled: vConfig.IsSet("resolver"),
		generatedBuildInfoPath:       vConfig.GetString(utils.GeneratedBuildInfo),
		mavenOpts:                    mavenOpts,
		deployableArtifactsFilePath:  vConfig.GetString(utils.DeployableArtifacts),
	}, nil
}

func setEmptyDeployer(vConfig *viper.Viper) {
	vConfig.Set(utils.DeployerPrefix+utils.DeployArtifacts, "false")
	vConfig.Set(utils.DeployerPrefix+utils.Url, "http://empty_url")
	vConfig.Set(utils.DeployerPrefix+utils.ReleaseRepo, "empty_repo")
	vConfig.Set(utils.DeployerPrefix+utils.SnapshotRepo, "empty_repo")
}

func setDeployFalse(vConfig *viper.Viper) {
	vConfig.Set(utils.DeployerPrefix+utils.DeployArtifacts, "false")
	if vConfig.GetString(utils.DeployerPrefix+utils.Url) == "" {
		vConfig.Set(utils.DeployerPrefix+utils.Url, "http://empty_url")
	}
	if vConfig.GetString(utils.DeployerPrefix+utils.ReleaseRepo) == "" {
		vConfig.Set(utils.DeployerPrefix+utils.ReleaseRepo, "empty_repo")
	}
	if vConfig.GetString(utils.DeployerPrefix+utils.SnapshotRepo) == "" {
		vConfig.Set(utils.DeployerPrefix+utils.SnapshotRepo, "empty_repo")
	}
}

func (config *mvnRunConfig) GetCmd() *exec.Cmd {
	var cmd []string
	cmd = append(cmd, config.java)
	cmd = append(cmd, "-classpath", config.plexusClassworlds)
	cmd = append(cmd, "-Dmaven.home="+config.mavenHome)
	cmd = append(cmd, "-DbuildInfoConfig.propertiesFile="+config.buildInfoProperties)
	if config.artifactoryResolutionEnabled {
		cmd = append(cmd, "-DbuildInfoConfig.artifactoryResolutionEnabled=true")
	}
	cmd = append(cmd, "-Dm3plugin.lib="+config.pluginDependencies)
	cmd = append(cmd, "-Dclassworlds.conf="+config.cleassworldsConfig)
	cmd = append(cmd, "-Dmaven.multiModuleProjectDirectory="+config.workspace)
	if config.mavenOpts != "" {
		cmd = append(cmd, strings.Split(config.mavenOpts, " ")...)
	}
	cmd = append(cmd, "org.codehaus.plexus.classworlds.launcher.Launcher")
	cmd = append(cmd, config.goals...)
	return exec.Command(cmd[0], cmd[1:]...)
}

type mvnRunConfig struct {
	java                         string
	plexusClassworlds            string
	cleassworldsConfig           string
	mavenHome                    string
	pluginDependencies           string
	workspace                    string
	pom                          string
	goals                        []string
	buildInfoProperties          string
	artifactoryResolutionEnabled bool
	generatedBuildInfoPath       string
	mavenOpts                    string
	deployableArtifactsFilePath  string
}

func (config *mvnRunConfig) runCmd() error {
	command := config.GetCmd()
	command.Stderr = os.Stderr
	command.Stdout = os.Stderr
	return coreutils.ConvertExitCodeError(errorutils.CheckError(command.Run()))
}
