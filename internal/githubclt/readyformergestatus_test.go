package githubclt

import (
	"testing"

	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/require"
)

func TestOverallCIStatus_optionalFailedChecksAreIgnored(t *testing.T) {
	status := overallCIStatus(
		githubv4.StatusStateError,
		[]*CIJobStatus{
			{
				Name:     "optional_check",
				Status:   CIStatusFailure,
				Required: false,
			},
			{
				Name:     "required_check",
				Status:   CIStatusSuccess,
				Required: true,
			},
		},
	)

	require.Equal(t, CIStatusSuccess, status)
}

func TestOverallCIStatus_optionalPendingChecksAreHonored(t *testing.T) {
	status := overallCIStatus(
		githubv4.StatusStateError,
		[]*CIJobStatus{
			{
				Name:     "optional_check",
				Status:   CIStatusPending,
				Required: false,
			},
			{
				Name:     "required_check",
				Status:   CIStatusSuccess,
				Required: true,
			},
		},
	)

	require.Equal(t, CIStatusPending, status)
}

func TestOverallCIStatus_requiredFailedCheck(t *testing.T) {
	status := overallCIStatus(
		githubv4.StatusStateError,
		[]*CIJobStatus{
			{
				Name:     "optional_check",
				Status:   CIStatusPending,
				Required: false,
			},
			{
				Name:     "required_check",
				Status:   CIStatusFailure,
				Required: true,
			},
			{
				Name:     "required_check1",
				Status:   CIStatusSuccess,
				Required: true,
			},
		},
	)

	require.Equal(t, CIStatusFailure, status)
}
