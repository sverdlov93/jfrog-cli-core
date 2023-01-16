package transferconfig

import (
	"github.com/jfrog/jfrog-client-go/access/services"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

const (
	quotaNumber = 1073741825
)

func TestCreateAndValidateConflicts(t *testing.T) {
	tests := []struct {
		sameKey           bool
		sameName          bool
		sameDescription   bool
		sameAdmin         bool
		sameQuotaBytes    bool
		sameSoftLimit     bool
		expectedDiffCount int
	}{
		{true, true, true, true, true, true, 0},
		{true, true, true, true, true, false, 1},
		{true, true, true, true, false, false, 2},
		{true, true, true, false, false, false, 4},
		{true, true, false, false, false, false, 5},
		{true, false, false, false, false, false, 6},
		{false, false, false, false, false, false, 7},
	}
	for _, test := range tests {
		source, target := createProjects(test.sameKey, test.sameName, test.sameDescription, test.sameAdmin, test.sameQuotaBytes, test.sameSoftLimit)
		conflicts := compareProjects(source, target)
		diffCount := 0
		if conflicts != nil {
			diffCount = len(strings.Split(conflicts.DifferentProperties, ";"))
		}
		assert.Equal(t, test.expectedDiffCount, diffCount)
	}
}

func createProjects(sameKey, sameName, sameDescription, sameAdmin, sameQuotaBytes, sameSoftLimit bool) (source, target services.Project) {
	sourceKey := "ProjectKey"
	targetKey := sourceKey
	sourceName := "ProjectName"
	targetName := sourceName
	sourceDescription := "ProjectDescription"
	targetDescription := sourceDescription
	sourceAdmin := &services.AdminPrivileges{}
	targetAdmin := &services.AdminPrivileges{}
	sourceQuotaBytes := float64(quotaNumber)
	targetQuotaBytes := float64(quotaNumber)
	if !sameKey {
		targetKey = sourceKey + "Target"
	}
	if !sameName {
		targetName = sourceName + "Target"
	}
	if !sameDescription {
		targetDescription = sourceDescription + "Target"
	}
	trueValue := true
	if !sameAdmin {
		targetAdmin.ManageMembers = &trueValue
		targetAdmin.IndexResources = &trueValue
	}
	var sourceSoftLimit *bool = nil
	var targetSoftLimit *bool = nil

	if !sameSoftLimit {
		targetSoftLimit = &trueValue
	}
	if !sameQuotaBytes {
		targetQuotaBytes = targetQuotaBytes + 125
	}
	source = services.Project{DisplayName: sourceName, Description: sourceDescription, AdminPrivileges: sourceAdmin, SoftLimit: sourceSoftLimit, StorageQuotaBytes: sourceQuotaBytes, ProjectKey: sourceKey}
	target = services.Project{DisplayName: targetName, Description: targetDescription, AdminPrivileges: targetAdmin, SoftLimit: targetSoftLimit, StorageQuotaBytes: targetQuotaBytes, ProjectKey: targetKey}
	return
}
