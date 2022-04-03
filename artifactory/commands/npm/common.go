package npm

import (
	"bufio"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	biutils "github.com/jfrog/build-info-go/build/utils"
	"github.com/jfrog/gofrog/version"
	commandUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/npm"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	npmConfigAuthEnv = "NPM_CONFIG__AUTH"
)

type CommonArgs struct {
	cmdName        string
	jsonOutput     bool
	executablePath string
	// Function to be called to restore the user's old npmrc and delete the one we created.
	restoreNpmrcFunc func() error
	workingDirectory string
	// Npm registry as exposed by Artifactory.
	registry string
	// Npm token generated by Artifactory using the user's provided credentials.
	npmAuth        string
	authArtDetails auth.ServiceDetails
	npmVersion     *version.Version
	NpmCommand
}

func (com *CommonArgs) preparePrerequisites(repo string, overrideNpmrc bool) error {
	log.Debug("Preparing prerequisites...")
	var err error
	com.npmVersion, com.executablePath, err = biutils.GetNpmVersionAndExecPath(log.Logger)
	if err != nil {
		return err
	}
	if !overrideNpmrc {
		return nil
	}
	if com.npmVersion.Compare(minSupportedNpmVersion) > 0 {
		return errorutils.CheckErrorf(
			"JFrog CLI npm %s command requires npm client version "+minSupportedNpmVersion+" or higher. The Current version is: %s", com.cmdName, com.npmVersion.GetVersion())
	}

	if err := com.setJsonOutput(); err != nil {
		return err
	}

	com.workingDirectory, err = coreutils.GetWorkingDirectory()
	if err != nil {
		return err
	}
	log.Debug("Working directory set to:", com.workingDirectory)
	if err = com.setArtifactoryAuth(); err != nil {
		return err
	}

	com.npmAuth, com.registry, err = commandUtils.GetArtifactoryNpmRepoDetails(repo, &com.authArtDetails)
	if err != nil {
		return err
	}

	return com.setRestoreNpmrcFunc()
}

func (com *CommonArgs) setJsonOutput() error {
	jsonOutput, err := npm.ConfigGet(com.npmArgs, "json", com.executablePath)
	if err != nil {
		return err
	}

	// In case of --json=<not boolean>, the value of json is set to 'true', but the result from the command is not 'true'
	com.jsonOutput = jsonOutput != "false"
	return nil
}

func (com *CommonArgs) setArtifactoryAuth() error {
	authArtDetails, err := com.serverDetails.CreateArtAuthConfig()
	if err != nil {
		return err
	}
	if authArtDetails.GetSshAuthHeaders() != nil {
		return errorutils.CheckErrorf("SSH authentication is not supported in this command")
	}
	com.authArtDetails = authArtDetails
	return nil
}

// In order to make sure the npm resolves artifacts from Artifactory we create a .npmrc file in the project dir.
// If such a file exists we back it up as npmrcBackupFileName.
func (com *CommonArgs) createTempNpmrc() error {
	log.Debug("Creating project .npmrc file.")
	data, err := npm.GetConfigList(com.npmArgs, com.executablePath)
	if err != nil {
		return err
	}
	configData, err := com.prepareConfigData(data)
	if err != nil {
		return errorutils.CheckError(err)
	}

	if err = removeNpmrcIfExists(com.workingDirectory); err != nil {
		return err
	}

	return errorutils.CheckError(ioutil.WriteFile(filepath.Join(com.workingDirectory, npmrcFileName), configData, 0600))
}

// This func transforms "npm config list" result to key=val list of values that can be set to .npmrc file.
// it filters out any nil value key, changes registry and scope registries to Artifactory url and adds Artifactory authentication to the list
func (com *CommonArgs) prepareConfigData(data []byte) ([]byte, error) {
	var filteredConf []string
	configString := string(data) + "\n" + com.npmAuth
	scanner := bufio.NewScanner(strings.NewReader(configString))

	for scanner.Scan() {
		currOption := scanner.Text()
		if currOption != "" {
			splitOption := strings.SplitN(currOption, "=", 2)
			key := strings.TrimSpace(splitOption[0])
			if len(splitOption) == 2 && isValidKey(key) {
				value := strings.TrimSpace(splitOption[1])
				if key == "_auth" {
					// Set "NPM_CONFIG__AUTH" environment variable to allow authentication with Artifactory when running postinstall scripts on subdirectories.
					if err := os.Setenv(npmConfigAuthEnv, value); err != nil {
						return nil, errorutils.CheckError(err)
					}
				} else if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
					filteredConf = addArrayConfigs(filteredConf, key, value)
				} else {
					filteredConf = append(filteredConf, currOption, "\n")
				}
			} else if strings.HasPrefix(splitOption[0], "@") {
				// Override scoped registries (@scope = xyz)
				filteredConf = append(filteredConf, splitOption[0], " = ", com.registry, "\n")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errorutils.CheckError(err)
	}

	filteredConf = append(filteredConf, "json = ", strconv.FormatBool(com.jsonOutput), "\n")
	filteredConf = append(filteredConf, "registry = ", com.registry, "\n")
	return []byte(strings.Join(filteredConf, "")), nil
}

func (com *CommonArgs) setRestoreNpmrcFunc() error {
	restoreNpmrcFunc, err := commandUtils.BackupFile(filepath.Join(com.workingDirectory, npmrcFileName), filepath.Join(com.workingDirectory, npmrcBackupFileName))
	if err != nil {
		return err
	}
	com.restoreNpmrcFunc = func() error {
		if unsetEnvErr := os.Unsetenv(npmConfigAuthEnv); unsetEnvErr != nil {
			log.Warn("Couldn't unset", npmConfigAuthEnv)
		}
		return restoreNpmrcFunc()
	}
	return err
}
