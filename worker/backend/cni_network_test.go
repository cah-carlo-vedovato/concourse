package backend_test

import (
	"context"
	"errors"

	"github.com/concourse/concourse/worker/backend"
	"github.com/concourse/concourse/worker/backend/backendfakes"
	"github.com/concourse/concourse/worker/backend/libcontainerd/libcontainerdfakes"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CNINetworkSuite struct {
	suite.Suite
	*require.Assertions

	network backend.Network
	cni     *backendfakes.FakeCNI
	store   *backendfakes.FakeFileStore
}

func (s *CNINetworkSuite) SetupTest() {
	var err error

	s.store = new(backendfakes.FakeFileStore)
	s.cni = new(backendfakes.FakeCNI)
	s.network, err = backend.NewCNINetwork(
		backend.WithCNIFileStore(s.store),
		backend.WithCNIClient(s.cni),
	)
	s.NoError(err)
}

func (s *CNINetworkSuite) TestNewCNINetworkWithInvalidConfigDoesntFail() {
	// CNI defers the actual interpretation of the network configuration to
	// the plugins.
	//
	_, err := backend.NewCNINetwork(
		backend.WithCNINetworkConfig(backend.CNINetworkConfig{
			Subnet: "_____________",
		}),
	)
	s.NoError(err)
}

func (s *CNINetworkSuite) TestSetupMountsEmptyHandle() {
	_, err := s.network.SetupMounts("")
	s.EqualError(err, "empty handle")
}

func (s *CNINetworkSuite) TestSetupMountsFailToCreateHosts() {
	s.store.CreateReturnsOnCall(0, "", errors.New("create-hosts-err"))

	_, err := s.network.SetupMounts("handle")
	s.EqualError(errors.Unwrap(err), "create-hosts-err")

	s.Equal(1, s.store.CreateCallCount())
	fname, _ := s.store.CreateArgsForCall(0)

	s.Equal("handle/hosts", fname)
}

func (s *CNINetworkSuite) TestSetupMountsFailToCreateResolvConf() {
	s.store.CreateReturnsOnCall(1, "", errors.New("create-resolvconf-err"))

	_, err := s.network.SetupMounts("handle")
	s.EqualError(errors.Unwrap(err), "create-resolvconf-err")

	s.Equal(2, s.store.CreateCallCount())
	fname, _ := s.store.CreateArgsForCall(1)

	s.Equal("handle/resolv.conf", fname)
}

func (s *CNINetworkSuite) TestSetupMountsReturnsMountpoints() {
	s.store.CreateReturnsOnCall(0, "/tmp/handle/etc/hosts", nil)
	s.store.CreateReturnsOnCall(1, "/tmp/handle/etc/resolv.conf", nil)

	mounts, err := s.network.SetupMounts("handle")
	s.NoError(err)

	s.Len(mounts, 2)
	s.Equal(mounts, []specs.Mount{
		{
			Destination: "/etc/hosts",
			Type:        "bind",
			Source:      "/tmp/handle/etc/hosts",
			Options:     []string{"bind", "rw"},
		},
		{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      "/tmp/handle/etc/resolv.conf",
			Options:     []string{"bind", "rw"},
		},
	})
}

func (s *CNINetworkSuite) TestAddNilTask() {
	err := s.network.Add(context.Background(), nil)
	s.EqualError(err, "nil task")
}

func (s *CNINetworkSuite) TestAddSetupErrors() {
	s.cni.SetupReturns(nil, errors.New("setup-err"))
	task := new(libcontainerdfakes.FakeTask)

	err := s.network.Add(context.Background(), task)
	s.EqualError(errors.Unwrap(err), "setup-err")
}

func (s *CNINetworkSuite) TestAdd() {
	task := new(libcontainerdfakes.FakeTask)
	task.PidReturns(123)
	task.IDReturns("id")

	err := s.network.Add(context.Background(), task)
	s.NoError(err)

	s.Equal(1, s.cni.SetupCallCount())
	_, id, netns, _ := s.cni.SetupArgsForCall(0)
	s.Equal("id", id)
	s.Equal("/proc/123/ns/net", netns)
}

func (s *CNINetworkSuite) TestRemoveNilTask() {
	err := s.network.Remove(context.Background(), nil)
	s.EqualError(err, "nil task")
}

func (s *CNINetworkSuite) TestRemoveSetupErrors() {
	s.cni.RemoveReturns(errors.New("remove-err"))
	task := new(libcontainerdfakes.FakeTask)

	err := s.network.Remove(context.Background(), task)
	s.EqualError(errors.Unwrap(err), "remove-err")
}

func (s *CNINetworkSuite) TestRemove() {
	task := new(libcontainerdfakes.FakeTask)
	task.PidReturns(123)
	task.IDReturns("id")

	err := s.network.Remove(context.Background(), task)
	s.NoError(err)

	s.Equal(1, s.cni.RemoveCallCount())
	_, id, netns, _ := s.cni.RemoveArgsForCall(0)
	s.Equal("id", id)
	s.Equal("/proc/123/ns/net", netns)
}
