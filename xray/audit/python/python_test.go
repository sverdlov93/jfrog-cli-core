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
	// Install virtualenv if missing
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
	// Remove virtualenv if exists
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
	rootNode, err := BuildDependencyTree(pythonutils.Pip, "")
	assert.NoError(t, err)
	assert.Len(t, rootNode, 1)
	if len(rootNode) > 0 {
		assert.NotEmpty(t, rootNode[0].Nodes)
		if rootNode[0].Nodes != nil {
			// Test direct dependency
			directDepNode := audit.GetAndAssertNode(t, rootNode[0].Nodes, "pip-example:1.2.3")
			// Test child module
			childNode := audit.GetAndAssertNode(t, directDepNode.Nodes, "pexpect:4.8.0")
			// Test sub child module
			audit.GetAndAssertNode(t, childNode.Nodes, "ptyprocess:0.7.0")
		}
	}
}

func TestPipDependencyListRequirementsFallback(t *testing.T) {
	// Create and change directory to test workspace
	_, cleanUp := audit.CreateTestWorkspace(t, filepath.Join("pip-project", "requirementsproject"))
	defer cleanUp()
	// No requirements file field specified, expect the command to use the fallback 'pip install -r requirements.txt' command
	rootNode, err := BuildDependencyTree(pythonutils.Pip, "")
	assert.NoError(t, err)
	assert.Len(t, rootNode, 1)
	if len(rootNode[0].Nodes) > 2 {
		childNode := audit.GetAndAssertNode(t, rootNode[0].Nodes, "pexpect:4.8.0")
		if childNode != nil {
			// Test child module
			audit.GetAndAssertNode(t, childNode.Nodes, "ptyprocess:0.7.0")
		}
	}
}

func TestBuildPipDependencyListRequirements(t *testing.T) {
	// Create and change directory to test workspace
	_, cleanUp := audit.CreateTestWorkspace(t, filepath.Join("pip-project", "requirementsproject"))
	defer cleanUp()
	// Run getModulesDependencyTrees
	rootNode, err := BuildDependencyTree(pythonutils.Pip, "requirements.txt")
	assert.NoError(t, err)
	assert.Len(t, rootNode, 1)
	if len(rootNode) > 0 {
		assert.NotEmpty(t, rootNode[0].Nodes)
		if rootNode[0].Nodes != nil {
			// Test root module
			directDepNode := audit.GetAndAssertNode(t, rootNode[0].Nodes, "pexpect:4.8.0")
			// Test child module
			audit.GetAndAssertNode(t, directDepNode.Nodes, "ptyprocess:0.7.0")
		}
	}
}

func TestBuildPipenvDependencyList(t *testing.T) {
	// Create and change directory to test workspace
	_, cleanUp := audit.CreateTestWorkspace(t, "pipenv-project")
	defer cleanUp()
	// Run getModulesDependencyTrees
	rootNode, err := BuildDependencyTree(pythonutils.Pipenv, "")
	if err != nil {
		t.Fatal(err)
	}
	assert.Len(t, rootNode, 1)
	if len(rootNode) > 0 {
		assert.NotEmpty(t, rootNode[0].Nodes)
		// Test child module
		childNode := audit.GetAndAssertNode(t, rootNode[0].Nodes, "pexpect:4.8.0")
		// Test sub child module
		if assert.NotNil(t, childNode) {
			audit.GetAndAssertNode(t, childNode.Nodes, "ptyprocess:0.7.0")
		}
	}
}

func TestBuildPoetryDependencyList(t *testing.T) {
	// Create and change directory to test workspace
	_, cleanUp := audit.CreateTestWorkspace(t, "poetry-project")
	defer cleanUp()
	// Run getModulesDependencyTrees
	rootNode, err := BuildDependencyTree(pythonutils.Poetry, "")
	if err != nil {
		t.Fatal(err)
	}
	assert.Len(t, rootNode, 1)
	if len(rootNode) > 0 {
		assert.NotEmpty(t, rootNode[0].Nodes)
		// Test child module
		childNode := audit.GetAndAssertNode(t, rootNode[0].Nodes, "pytest:5.4.3")
		// Test sub child module
		if assert.NotNil(t, childNode) {
			transitiveChildNode := audit.GetAndAssertNode(t, childNode.Nodes, "packaging:21.3")
			if assert.NotNil(t, transitiveChildNode) {
				audit.GetAndAssertNode(t, transitiveChildNode.Nodes, "pyparsing:3.0.9")
			}
		}
	}
}
