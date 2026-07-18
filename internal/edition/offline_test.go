//go:build offline

package edition_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frp-panel/frp-panel/internal/config"
	"github.com/frp-panel/frp-panel/internal/edition"
	updateservice "github.com/frp-panel/frp-panel/internal/service/update"
)

func TestOfflineApplyDisablesAllUpdateCenterTraffic(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	cfg := &config.Config{Update: config.UpdateConfig{
		Enabled: true, CenterURL: server.URL, PanelVersion: "9.9.9", PanelDomain: "panel.example.com",
		HeartbeatEnabled: true, AnonymousStatistics: true, BootstrapURLs: []string{server.URL + "/bootstrap.json"},
	}}
	edition.Apply(cfg)
	if !edition.Offline || !reflect.DeepEqual(cfg.Update, config.UpdateConfig{}) {
		t.Fatalf("offline update configuration was not cleared: %#v", cfg.Update)
	}

	client := updateservice.NewClient(cfg.Update, cfg.Update.InstanceID)
	client.Start(context.Background())
	defer client.Stop()
	result, err := client.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Enabled {
		t.Fatal("offline update check reported enabled")
	}
	time.Sleep(50 * time.Millisecond)
	if got := requests.Load(); got != 0 {
		t.Fatalf("offline update client made %d outbound requests", got)
	}
}
