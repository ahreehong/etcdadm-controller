package controllers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/aws/etcdadm-controller/controllers/mocks"
	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/types"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestStartHealthCheckLoop(t *testing.T) {
	_ = NewWithT(t)

	ctrl := gomock.NewController(t)
	mockEtcd := mocks.NewMockEtcdClient(ctrl)
	mockRt := mocks.NewMockRoundTripper(ctrl, "")

	etcdTest := newEtcdadmClusterTest(3)
	etcdTest.buildClusterWithExternalEtcd()
	etcdTest.etcdadmCluster.Status.CreationComplete = true

	fakeKubernetesClient := fake.NewClientBuilder().WithScheme(setupScheme()).WithObjects(etcdTest.gatherObjects()...).Build()

	etcdEtcdClient := func(ctx context.Context, cluster *clusterv1.Cluster, endpoints string) (EtcdClient, error) {
		return mockEtcd, nil
	}

	r := &EtcdadmClusterReconciler{
		Client:         fakeKubernetesClient,
		uncachedClient: fakeKubernetesClient,
		Log:            log.Log,
		GetEtcdClient:  etcdEtcdClient,
	}
	mockHttpClient := &http.Client{
		Transport: mockRt,
	}

	r.etcdHealthCheckConfig.clusterToHttpClient.Store(etcdTest.cluster.UID, mockHttpClient)
	r.SetIsPortOpen(isPortOpenMock)

	mockRt.EXPECT().RoundTrip(gomock.Any()).Return(getHealthyEtcdHttpResponse(), nil).Times(3)

	etcdadmClusterMapper := make(map[types.UID]etcdadmClusterMemberHealthConfig)
	r.startHealthCheck(context.Background(), etcdadmClusterMapper)
}

func isPortOpenMock(_ context.Context, _ string) bool {
	return true
}

func TestReconcilePeriodicHealthCheckMachineToBeDeletedNowHealthy(t *testing.T) {
	g := NewWithT(t)

	ctrl := gomock.NewController(t)
	mockEtcd := mocks.NewMockEtcdClient(ctrl)
	mockRt := mocks.NewMockRoundTripper(ctrl, "")

	etcdadmCluster := newEtcdadmClusterTest(1)
	etcdadmCluster.buildClusterWithExternalEtcd()
	etcdadmCluster.etcdadmCluster.Status.CreationComplete = true

	fakeKubernetesClient := fake.NewClientBuilder().WithScheme(setupScheme()).WithObjects(etcdadmCluster.gatherObjects()...).Build()

	etcdEtcdClient := func(ctx context.Context, cluster *clusterv1.Cluster, endpoints string) (EtcdClient, error) {
		return mockEtcd, nil
	}

	r := &EtcdadmClusterReconciler{
		Client:         fakeKubernetesClient,
		uncachedClient: fakeKubernetesClient,
		Log:            log.Log,
		GetEtcdClient:  etcdEtcdClient,
	}
	mockHttpClient := &http.Client{
		Transport: mockRt,
	}

	r.etcdHealthCheckConfig.clusterToHttpClient.Store(etcdadmCluster.cluster.UID, mockHttpClient)
	r.SetIsPortOpen(isPortOpenMock)

	etcdadmClusterMapper := make(map[types.UID]etcdadmClusterMemberHealthConfig)

	for i := 0; i < 5; i++ {
		mockRt.EXPECT().RoundTrip(gomock.Any()).Return(nil, errors.New("error"))
		mockEtcd.EXPECT().MemberList(gomock.Any())
		mockEtcd.EXPECT().Close()
		r.startHealthCheck(context.Background(), etcdadmClusterMapper)
	}

	unhealthyList := len(etcdadmClusterMapper[etcdadmCluster.etcdadmCluster.UID].unhealthyMembersToRemove)
	g.Expect(unhealthyList).To(Equal(1))

	mockRt.EXPECT().RoundTrip(gomock.Any()).Return(getHealthyEtcdHttpResponse(), nil)
	r.startHealthCheck(context.Background(), etcdadmClusterMapper)
	unhealthyList = len(etcdadmClusterMapper[etcdadmCluster.etcdadmCluster.UID].unhealthyMembersToRemove)

	g.Expect(unhealthyList).To(Equal(0))
}

func TestQuorumNotPreserved(t *testing.T) {
	g := NewWithT(t)

	ctrl := gomock.NewController(t)
	mockEtcd := mocks.NewMockEtcdClient(ctrl)
	mockRt := mocks.NewMockRoundTripper(ctrl, "")

	etcdTest := newEtcdadmClusterTest(3)
	etcdTest.buildClusterWithExternalEtcd()
	etcdTest.etcdadmCluster.Status.CreationComplete = true

	fakeKubernetesClient := fake.NewClientBuilder().WithScheme(setupScheme()).WithObjects(etcdTest.gatherObjects()...).Build()

	etcdEtcdClient := func(ctx context.Context, cluster *clusterv1.Cluster, endpoints string) (EtcdClient, error) {
		return mockEtcd, nil
	}

	r := &EtcdadmClusterReconciler{
		Client:         fakeKubernetesClient,
		uncachedClient: fakeKubernetesClient,
		Log:            log.Log,
		GetEtcdClient:  etcdEtcdClient,
	}
	mockHttpClient := &http.Client{
		Transport: mockRt,
	}

	r.etcdHealthCheckConfig.clusterToHttpClient.Store(etcdTest.cluster.UID, mockHttpClient)
	r.SetIsPortOpen(isPortOpenMock)

	etcdadmClusterMapper := make(map[types.UID]etcdadmClusterMemberHealthConfig)

	for i := 0; i < 5; i++ {
		mockRt.EXPECT().RoundTrip(gomock.Any()).Return(nil, errors.New("error")).Times(3)
		mockEtcd.EXPECT().MemberList(gomock.Any())
		mockEtcd.EXPECT().Close()
		r.startHealthCheck(context.Background(), etcdadmClusterMapper)
	}

	unhealthyList := len(etcdadmClusterMapper[etcdTest.etcdadmCluster.UID].unhealthyMembersToRemove)
	g.Expect(unhealthyList).To(Equal(3))

	for i := 0; i < 3; i++ {
		mockRt.EXPECT().RoundTrip(gomock.Any()).Return(nil, errors.New("error")).Times(3)
		mockEtcd.EXPECT().MemberList(gomock.Any())
		mockEtcd.EXPECT().Close()
		r.startHealthCheck(context.Background(), etcdadmClusterMapper)
	}

	unhealthyList = len(etcdadmClusterMapper[etcdTest.etcdadmCluster.UID].unhealthyMembersToRemove)
	machineList := &clusterv1.MachineList{}
	g.Expect(fakeKubernetesClient.List(context.Background(), machineList)).To(Succeed())
	for _, m := range machineList.Items {
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
	}
	g.Expect(unhealthyList).To(Equal(3))
}

func TestQuorumPreserved(t *testing.T) {
	g := NewWithT(t)

	ctrl := gomock.NewController(t)
	mockEtcd := mocks.NewMockEtcdClient(ctrl)

	etcdTest := newEtcdadmClusterTest(3)
	etcdTest.buildClusterWithExternalEtcd()
	etcdTest.etcdadmCluster.Status.CreationComplete = true

	ip := etcdTest.machines[0].Status.Addresses[0].Address

	mockRt := mocks.NewMockRoundTripper(ctrl, ip)

	fakeKubernetesClient := fake.NewClientBuilder().WithScheme(setupScheme()).WithObjects(etcdTest.gatherObjects()...).Build()

	etcdEtcdClient := func(ctx context.Context, cluster *clusterv1.Cluster, endpoints string) (EtcdClient, error) {
		return mockEtcd, nil
	}

	r := &EtcdadmClusterReconciler{
		Client:         fakeKubernetesClient,
		uncachedClient: fakeKubernetesClient,
		Log:            log.Log,
		GetEtcdClient:  etcdEtcdClient,
	}
	mockHttpClient := &http.Client{
		Transport: mockRt,
	}

	r.etcdHealthCheckConfig.clusterToHttpClient.Store(etcdTest.cluster.UID, mockHttpClient)
	r.SetIsPortOpen(isPortOpenMock)

	etcdadmClusterMapper := make(map[types.UID]etcdadmClusterMemberHealthConfig)

	for i := 0; i < 5; i++ {
		gomock.InOrder(
			mockRt.EXPECT().RoundTrip(gomock.Any()).Return(nil, errors.New("error")).Times(1),
			mockRt.EXPECT().RoundTrip(gomock.Any()).Return(getHealthyEtcdHttpResponse(), nil),
			mockRt.EXPECT().RoundTrip(gomock.Any()).Return(getHealthyEtcdHttpResponse(), nil),
		)
		mockEtcd.EXPECT().MemberList(gomock.Any())
		mockEtcd.EXPECT().Close()
		r.startHealthCheck(context.Background(), etcdadmClusterMapper)
	}

	unhealthyList := len(etcdadmClusterMapper[etcdTest.etcdadmCluster.UID].unhealthyMembersToRemove)
	g.Expect(unhealthyList).To(Equal(1))

	mockRt.EXPECT().RoundTrip(gomock.Any()).Return(nil, errors.New("error")).Times(1)
	mockRt.EXPECT().RoundTrip(gomock.Any()).Return(getHealthyEtcdHttpResponse(), nil)
	mockRt.EXPECT().RoundTrip(gomock.Any()).Return(getHealthyEtcdHttpResponse(), nil)
	mockEtcd.EXPECT().MemberList(gomock.Any())
	mockEtcd.EXPECT().Close()
	r.startHealthCheck(context.Background(), etcdadmClusterMapper)

	unhealthyList = len(etcdadmClusterMapper[etcdTest.etcdadmCluster.UID].unhealthyMembersToRemove)
	machineList := &clusterv1.MachineList{}
	g.Expect(fakeKubernetesClient.List(context.Background(), machineList)).To(Succeed())
	g.Expect(machineList.Items[0].DeletionTimestamp.IsZero()).To(BeFalse())
	g.Expect(unhealthyList).To(Equal(0))
}
