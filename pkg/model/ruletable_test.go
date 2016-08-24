package model

import (
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo"
	sd "github.com/labstack/echo/engine/standard"
)

const (
	etcdAddr   = "http://192.168.70.13:2379"
	etcdPrefix = "/gateway2"
)

var (
	serverAddr    = "127.0.0.1:12345"
	angUrl        = "/api/test"
	checkDuration = 3
	checkTimeout  = 2
	clusterName   = "app"
	lbName        = "ROUNDROBIN"
	sleep         = false
)

var rt *RouteTable

func createRouteTable(t *testing.T) {
	store, err := NewEtcdStore([]string{etcdAddr}, etcdPrefix)

	if nil != err {
		t.Fatalf("create etcd store err.addr:<%s>", err)
	}

	store.Clean()

	rt = NewRouteTable(store)
	time.Sleep(time.Second * 1)
}

func createLocalServer() {
	e := echo.New()

	e.Get("/check", func() echo.HandlerFunc {
		return func(c echo.Context) error {
			if sleep {
				time.Sleep(time.Second * time.Duration(checkTimeout+1))
			}

			return c.String(http.StatusOK, "OK")
		}
	}())

	e.Run(sd.New(serverAddr))
}

func waitNotify() {
	time.Sleep(time.Second * 1)
}

func TestCreateRouteTable(t *testing.T) {
	createRouteTable(t)
}

func TestEtcdWatchNewServer(t *testing.T) {
	go createLocalServer()

	server := &Server{
		Schema:          "http",
		Addr:            serverAddr,
		CheckPath:       "/check",
		CheckDuration:   checkDuration,
		CheckTimeout:    checkTimeout,
		MaxQPS:          1500,
		HalfToOpen:      10,
		HalfTrafficRate: 10,
		CloseCount:      100,
	}

	err := rt.store.SaveServer(server)

	if nil != err {
		t.Error("add server err.")
		return
	}

	waitNotify()

	if len(rt.svrs) != 1 {
		t.Errorf("expect:<1>, acture:<%d>", len(rt.svrs))
		return
	}

	if rt.svrs[serverAddr].lock == nil {
		t.Error("server init error.")
		return
	}
}

func TestServerCheckOk(t *testing.T) {
	time.Sleep(time.Second * time.Duration(checkDuration))

	if rt.svrs[serverAddr].Status == DOWN {
		t.Errorf("status check ok err.expect:<UP>, acture:<%v>", DOWN)
	}
}

func TestServerCheckTimeout(t *testing.T) {
	defer func() {
		sleep = false
	}()

	sleep = true
	time.Sleep(time.Second * time.Duration(checkDuration*2+1)) // 等待两个周期

	if rt.svrs[serverAddr].Status == UP {
		t.Errorf("status check timeout err.expect:<DOWN>, acture:<%v>", UP)
		return
	}
}

func TestServerCheckTimeoutRecovery(t *testing.T) {
	time.Sleep(time.Second * time.Duration(checkDuration*2+1)) // 等待两个周期

	if rt.svrs[serverAddr].Status == DOWN {
		t.Errorf("status check timeout recovery err.expect:<UP>, acture:<%v>", UP)
		return
	}
}

func TestEtcdWatchNewCluster(t *testing.T) {
	cluster := &Cluster{
		Name:    clusterName,
		Pattern: "/api/*",
		LbName:  lbName,
	}

	err := rt.store.SaveCluster(cluster)

	if nil != err {
		t.Error("add cluster err.")
		return
	}

	waitNotify()

	if len(rt.clusters) == 1 {
		return
	}

	t.Errorf("expect:<1>, acture:<%d>", len(rt.clusters))
}

func TestEtcdWatchNewBind(t *testing.T) {
	bind := &Bind{
		ClusterName: clusterName,
		ServerAddr:  serverAddr,
	}

	err := rt.store.SaveBind(bind)

	if nil != err {
		t.Error("add cluster err.")
		return
	}

	waitNotify()

	if len(rt.mapping) == 1 {
		return
	}

	t.Errorf("expect:<1>, acture:<%d>. %+v", len(rt.mapping), rt.mapping)
}

func TestEtcdWatchNewAggregation(t *testing.T) {
	n := &Node{
		AttrName:    "test",
		URL:         "/api/node/test",
		ClusterName: clusterName,
	}

	err := rt.store.SaveAggregation(&Aggregation{
		URL:   angUrl,
		Nodes: []*Node{n},
	})

	if nil != err {
		t.Error("add aggregation err.")
		return
	}

	waitNotify()

	if len(rt.aggregations) == 1 {
		return
	}

	t.Errorf("expect:<1>, acture:<%d>", len(rt.aggregations))
}

func TestEtcdWatchUpdateServer(t *testing.T) {
	server := &Server{
		Schema:          "http",
		Addr:            serverAddr,
		CheckPath:       "/check",
		CheckDuration:   checkDuration,
		CheckTimeout:    checkTimeout * 2,
		MaxQPS:          3000,
		HalfToOpen:      100,
		HalfTrafficRate: 30,
		CloseCount:      200,
	}

	err := rt.store.UpdateServer(server)

	if nil != err {
		t.Error("update server err.")
		return
	}

	waitNotify()

	svr := rt.svrs[serverAddr]

	if svr.MaxQPS != server.MaxQPS {
		t.Errorf("MaxQPS expect:<%d>, acture:<%d>. ", server.MaxQPS, svr.MaxQPS)
		return
	}

	if svr.HalfToOpen != server.HalfToOpen {
		t.Errorf("HalfToOpen expect:<%d>, acture:<%d>. ", server.HalfToOpen, svr.HalfToOpen)
		return
	}

	if svr.HalfTrafficRate != server.HalfTrafficRate {
		t.Errorf("HalfTrafficRate expect:<%d>, acture:<%d>. ", server.HalfTrafficRate, svr.HalfTrafficRate)
		return
	}

	if svr.CloseCount != server.CloseCount {
		t.Errorf("CloseCount expect:<%d>, acture:<%d>. ", server.CloseCount, svr.CloseCount)
		return
	}

	if svr.CheckTimeout == server.CheckTimeout {
		t.Errorf("CheckTimeout expect:<%d>, acture:<%d>. ", svr.CheckTimeout, server.CheckTimeout)
		return
	}
}

func TestEtcdWatchUpdateCluster(t *testing.T) {
	cluster := &Cluster{
		Name:    clusterName,
		Pattern: "/api/new/*",
		LbName:  lbName,
	}

	err := rt.store.UpdateCluster(cluster)

	if nil != err {
		t.Error("update cluster err.")
		return
	}

	waitNotify()

	existCluster := rt.clusters[clusterName]

	if existCluster.Pattern != cluster.Pattern {
		t.Errorf("Pattern expect:<%s>, acture:<%s>. ", cluster.Pattern, existCluster.Pattern)
		return
	}

	if existCluster.LbName != cluster.LbName {
		t.Errorf("LbName expect:<%s>, acture:<%s>. ", cluster.LbName, existCluster.LbName)
		return
	}
}

func TestEtcdWatchUpdateAggregation(t *testing.T) {
	n := &Node{
		AttrName:    "test",
		URL:         "/api/node/test",
		ClusterName: clusterName,
	}

	n2 := &Node{
		AttrName:    "tes2t",
		URL:         "/api/node/test2",
		ClusterName: clusterName,
	}

	ang := &Aggregation{
		URL:   angUrl,
		Nodes: []*Node{n, n2},
	}

	err := rt.store.UpdateAggregation(ang)

	if nil != err {
		t.Error("update aggregation err.")
		return
	}

	waitNotify()

	existAng, _ := rt.aggregations[ang.URL]

	if len(existAng.Nodes) != len(ang.Nodes) {
		t.Errorf("Nodes expect:<%s>, acture:<%s>. ", len(existAng.Nodes), len(ang.Nodes))
		return
	}
}

func TestEtcdWatchDeleteCluster(t *testing.T) {
	err := rt.store.DeleteCluster(clusterName)

	if nil != err {
		t.Error("delete cluster err.")
		return
	}

	waitNotify()

	if len(rt.clusters) != 0 {
		t.Errorf("clusters expect:<0>, acture:<%d>", len(rt.clusters))
		return
	}

	banded, _ := rt.mapping[serverAddr]

	if len(banded) != 0 {
		t.Errorf("banded expect:<0>, acture:<%d>", len(banded))
		return
	}
}

func TestEtcdWatchDeleteServer(t *testing.T) {
	err := rt.store.DeleteServer(serverAddr)

	if nil != err {
		t.Error("delete server err.")
		return
	}

	waitNotify()

	if len(rt.svrs) != 0 {
		t.Errorf("svrs expect:<0>, acture:<%d>", len(rt.svrs))
		return
	}

	if len(rt.mapping) != 0 {
		t.Errorf("mapping expect:<0>, acture:<%d>", len(rt.mapping))
		return
	}
}

func TestEtcdWatchDeleteAggregation(t *testing.T) {
	err := rt.store.DeleteAggregation(angUrl)

	if nil != err {
		t.Error("delete aggregation err.")
		return
	}

	waitNotify()

	if len(rt.aggregations) != 0 {
		t.Errorf("aggregations expect:<0>, acture:<%d>", len(rt.aggregations))
		return
	}
}

func TestEtcdWatchNewRouting(t *testing.T) {
	r, err := NewRouting(`desc = "test"; deadline = 100; rule = ["$query_abc == 10", "$query_123 == 20"];`, clusterName, "")

	if nil != err {
		t.Error("add routing err.")
		return
	}

	err = rt.store.SaveRouting(r)

	if nil != err {
		t.Error("add routing err.")
		return
	}

	waitNotify()

	if len(rt.routings) == 1 {
		delete(rt.routings, r.ID)
		return
	}

	t.Errorf("expect:<1>, acture:<%d>", len(rt.routings))
}

func TestEtcdWatchDeleteRouting(t *testing.T) {
	r, err := NewRouting(`desc = "test"; deadline = 3; rule = ["$query_abc == 10", "$query_123 == 20"];`, clusterName, "")

	if nil != err {
		t.Error("add routing err.")
		return
	}

	err = rt.store.SaveRouting(r)

	if nil != err {
		t.Error("add routing err.")
		return
	}

	time.Sleep(time.Second * 30)

	if len(rt.routings) == 0 {
		return
	}

	t.Errorf("expect:<0>, acture:<%d>", len(rt.routings))
}
