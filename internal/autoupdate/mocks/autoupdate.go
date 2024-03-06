// Code generated by MockGen. DO NOT EDIT.
// Source: internal/autoupdate/autoupdate.go
//
// Generated by this command:
//
//	mockgen -package mocks -source internal/autoupdate/autoupdate.go -destination internal/autoupdate/mocks/autoupdate.go
//

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	githubclt "github.com/simplesurance/goordinator/internal/githubclt"
	gomock "go.uber.org/mock/gomock"
	zap "go.uber.org/zap"
)

// MockGithubClient is a mock of GithubClient interface.
type MockGithubClient struct {
	ctrl     *gomock.Controller
	recorder *MockGithubClientMockRecorder
}

// MockGithubClientMockRecorder is the mock recorder for MockGithubClient.
type MockGithubClientMockRecorder struct {
	mock *MockGithubClient
}

// NewMockGithubClient creates a new mock instance.
func NewMockGithubClient(ctrl *gomock.Controller) *MockGithubClient {
	mock := &MockGithubClient{ctrl: ctrl}
	mock.recorder = &MockGithubClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockGithubClient) EXPECT() *MockGithubClientMockRecorder {
	return m.recorder
}

// AddLabel mocks base method.
func (m *MockGithubClient) AddLabel(ctx context.Context, owner, repo string, pullRequestOrIssueNumber int, label string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddLabel", ctx, owner, repo, pullRequestOrIssueNumber, label)
	ret0, _ := ret[0].(error)
	return ret0
}

// AddLabel indicates an expected call of AddLabel.
func (mr *MockGithubClientMockRecorder) AddLabel(ctx, owner, repo, pullRequestOrIssueNumber, label any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddLabel", reflect.TypeOf((*MockGithubClient)(nil).AddLabel), ctx, owner, repo, pullRequestOrIssueNumber, label)
}

// CreateIssueComment mocks base method.
func (m *MockGithubClient) CreateIssueComment(ctx context.Context, owner, repo string, issueOrPRNr int, comment string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateIssueComment", ctx, owner, repo, issueOrPRNr, comment)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreateIssueComment indicates an expected call of CreateIssueComment.
func (mr *MockGithubClientMockRecorder) CreateIssueComment(ctx, owner, repo, issueOrPRNr, comment any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateIssueComment", reflect.TypeOf((*MockGithubClient)(nil).CreateIssueComment), ctx, owner, repo, issueOrPRNr, comment)
}

// ListPullRequests mocks base method.
func (m *MockGithubClient) ListPullRequests(ctx context.Context, owner, repo, state, sort, sortDirection string) githubclt.PRIterator {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListPullRequests", ctx, owner, repo, state, sort, sortDirection)
	ret0, _ := ret[0].(githubclt.PRIterator)
	return ret0
}

// ListPullRequests indicates an expected call of ListPullRequests.
func (mr *MockGithubClientMockRecorder) ListPullRequests(ctx, owner, repo, state, sort, sortDirection any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListPullRequests", reflect.TypeOf((*MockGithubClient)(nil).ListPullRequests), ctx, owner, repo, state, sort, sortDirection)
}

// ReadyForMerge mocks base method.
func (m *MockGithubClient) ReadyForMerge(ctx context.Context, owner, repo string, prNumber int) (*githubclt.ReadyForMergeStatus, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadyForMerge", ctx, owner, repo, prNumber)
	ret0, _ := ret[0].(*githubclt.ReadyForMergeStatus)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReadyForMerge indicates an expected call of ReadyForMerge.
func (mr *MockGithubClientMockRecorder) ReadyForMerge(ctx, owner, repo, prNumber any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadyForMerge", reflect.TypeOf((*MockGithubClient)(nil).ReadyForMerge), ctx, owner, repo, prNumber)
}

// RemoveLabel mocks base method.
func (m *MockGithubClient) RemoveLabel(ctx context.Context, owner, repo string, pullRequestOrIssueNumber int, label string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RemoveLabel", ctx, owner, repo, pullRequestOrIssueNumber, label)
	ret0, _ := ret[0].(error)
	return ret0
}

// RemoveLabel indicates an expected call of RemoveLabel.
func (mr *MockGithubClientMockRecorder) RemoveLabel(ctx, owner, repo, pullRequestOrIssueNumber, label any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoveLabel", reflect.TypeOf((*MockGithubClient)(nil).RemoveLabel), ctx, owner, repo, pullRequestOrIssueNumber, label)
}

// UpdateBranch mocks base method.
func (m *MockGithubClient) UpdateBranch(ctx context.Context, owner, repo string, pullRequestNumber int) (bool, bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateBranch", ctx, owner, repo, pullRequestNumber)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(bool)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// UpdateBranch indicates an expected call of UpdateBranch.
func (mr *MockGithubClientMockRecorder) UpdateBranch(ctx, owner, repo, pullRequestNumber any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateBranch", reflect.TypeOf((*MockGithubClient)(nil).UpdateBranch), ctx, owner, repo, pullRequestNumber)
}

// MockRetryer is a mock of Retryer interface.
type MockRetryer struct {
	ctrl     *gomock.Controller
	recorder *MockRetryerMockRecorder
}

// MockRetryerMockRecorder is the mock recorder for MockRetryer.
type MockRetryerMockRecorder struct {
	mock *MockRetryer
}

// NewMockRetryer creates a new mock instance.
func NewMockRetryer(ctrl *gomock.Controller) *MockRetryer {
	mock := &MockRetryer{ctrl: ctrl}
	mock.recorder = &MockRetryerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRetryer) EXPECT() *MockRetryerMockRecorder {
	return m.recorder
}

// Run mocks base method.
func (m *MockRetryer) Run(arg0 context.Context, arg1 func(context.Context) error, arg2 []zap.Field) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Run", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// Run indicates an expected call of Run.
func (mr *MockRetryerMockRecorder) Run(arg0, arg1, arg2 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Run", reflect.TypeOf((*MockRetryer)(nil).Run), arg0, arg1, arg2)
}
