package python

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jfrog/build-info-go/utils/pythonutils"
	"github.com/jfrog/jfrog-cli-core/v2/xray/audit"
	"github.com/stretchr/testify/assert"
)

func TestBuildPipDependencyListSetuppyWithVirtualenv(t *testing.T) {
	path, _ := exec.LookPath("virtualenv")
	if path == "" {
		assert.NoError(t, executeCommand("pip", "install", "virtualenv"))
		defer func() {
			assert.NoError(t, executeCommand("pip", "uninstall", "virtualenv", "-y"))
		}()
	}
	testBuildPipDependencyListSetuppy(t)
}

func TestBuildPipDependencyListSetuppyWithPython3Venv(t *testing.T) {
	path, _ := exec.LookPath("virtualenv")
	if path != "" {
		assert.NoError(t, executeCommand("pip", "uninstall", "virtualenv", "-y"))
		defer func() {
			assert.NoError(t, executeCommand("pip", "install", "virtualenv"))
		}()
	}
	testBuildPipDependencyListSetuppy(t)
}

func testBuildPipDependencyListSetuppy(t *testing.T) {
	// Create and change directory to test workspace
	_, cleanUp := audit.CreateTestWorkspace(t, filepath.Join("pip-project", "setuppyproject"))
	defer cleanUp()
	// Run getModulesDependencyTrees
	rootNodes, err := BuildDependencyTree(pythonutils.Pip, "")
	if assert.NoError(t, err) && assert.NotEmpty(t, rootNodes) {
		// Test root module
		rootNode := audit.GetAndAssertNode(t, rootNodes, "pip-example:1.2.3")
		if rootNode != nil {
			// Test child module
			childNode := audit.GetAndAssertNode(t, rootNode.Nodes, "pexpect:4.8.0")
			// Test sub child module
			audit.GetAndAssertNode(t, childNode.Nodes, "ptyprocess:0.7.0")
		}
	}
}

func TestBuildPipDependencyListRequirements(t *testing.T) {
	// Create and change directory to test workspace
	_, cleanUp := audit.CreateTestWorkspace(t, filepath.Join("pip-project", "requirementsproject"))
	defer cleanUp()
	// Run getModulesDependencyTrees
	rootNodes, err := BuildDependencyTree(pythonutils.Pip, "requirements.txt")
	if assert.NoError(t, err) && assert.NotEmpty(t, rootNodes) {
		// Test root module
		rootNode := audit.GetAndAssertNode(t, rootNodes, "pexpect:4.8.0")
		// Test child module
		audit.GetAndAssertNode(t, rootNode.Nodes, "ptyprocess:0.7.0")
	}
}

func TestBuildPipenvDependencyList(t *testing.T) {
	// Create and change directory to test workspace
	_, cleanUp := audit.CreateTestWorkspace(t, "pipenv-project")
	defer cleanUp()
	// Run getModulesDependencyTrees
	rootNodes, err := BuildDependencyTree(pythonutils.Pipenv, "")
	if assert.NoError(t, err) && assert.NotEmpty(t, rootNodes) {
		// Test child module
		childNode := audit.GetAndAssertNode(t, rootNodes, "pexpect:4.8.0")
		// Test sub child module
		audit.GetAndAssertNode(t, childNode.Nodes, "ptyprocess:0.7.0")
	}
}

func TestBuildPoetryDependencyList(t *testing.T) {
	// Create and change directory to test workspace
	_, cleanUp := audit.CreateTestWorkspace(t, "poetry-project")
	defer cleanUp()
	// Run getModulesDependencyTrees
	rootNodes, err := BuildDependencyTree(pythonutils.Poetry, "")
	if assert.NoError(t, err) && assert.NotEmpty(t, rootNodes) {
		// Test child module
		childNode := audit.GetAndAssertNode(t, rootNodes, "pytest:5.4.3")
		// Test sub child module
		if assert.NotNil(t, childNode) {
			transitiveChildNode := audit.GetAndAssertNode(t, childNode.Nodes, "packaging:21.3")
			if assert.NotNil(t, transitiveChildNode) {
				audit.GetAndAssertNode(t, transitiveChildNode.Nodes, "pyparsing:3.0.9")
			}
		}
	}
}
