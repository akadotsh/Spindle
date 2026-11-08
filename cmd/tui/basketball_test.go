package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/akashprasad/file-uploader/packages/sdk"
)

func TestShotOnlyUploadsAfterBasket(t *testing.T) {
	for _, tc := range []struct {
		name       string
		aimX, aimY int
		wantScreen screen
	}{
		{"basket near rim", 32, 6, uploadingScreen},
		{"miss", 20, 5, shotScreen},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "test.txt")
			if err := os.WriteFile(source, []byte("test"), 0o600); err != nil {
				t.Fatal(err)
			}
			m := initialModel()
			m.provider = sdk.ProviderFilesystem
			m.screen = shotScreen
			m.fields = uploadFieldsFor(m.provider)
			m.fields[0].input.SetValue(source)
			m.fields[1].input.SetValue("local")
			m.fields[2].input.SetValue("test.txt")
			m.shot = newShotState()
			m.shot.aimX, m.shot.aimY = tc.aimX, tc.aimY
			updated, _ := m.launchShot()
			m = updated.(model)
			for frame := 0; frame < 17 && m.screen == shotScreen; frame++ {
				var cmd tea.Cmd
				updated, cmd = m.shotTick(shotTickMsg(m.shot.id))
				m = updated.(model)
				if m.screen == uploadingScreen && cmd == nil {
					t.Fatal("basket did not start an upload")
				}
			}
			if m.screen != tc.wantScreen {
				t.Fatalf("screen = %v, want %v", m.screen, tc.wantScreen)
			}
			if tc.wantScreen == shotScreen && (m.shot.flying || m.shot.status == "") {
				t.Fatal("miss did not reset for another attempt")
			}
		})
	}
}

func TestMouseDragAimsShot(t *testing.T) {
	m := initialModel()
	m.screen = shotScreen
	m.shot = newShotState()
	updated, _ := m.shotClick(tea.MouseClickMsg{X: shotLeft + ballX, Y: shotTop + ballY, Button: tea.MouseLeft})
	m = updated.(model)
	if !m.shot.dragging {
		t.Fatal("clicking the ball did not begin aiming")
	}
	updated, _ = m.shotMotion(tea.MouseMotionMsg{X: shotLeft + 32, Y: shotTop + 6, Button: tea.MouseLeft})
	m = updated.(model)
	updated, _ = m.shotRelease(tea.MouseReleaseMsg{X: shotLeft + 32, Y: shotTop + 6, Button: tea.MouseLeft})
	m = updated.(model)
	if m.screen != shotScreen || !m.shot.flying || !m.shot.scored || m.shot.dragging {
		t.Fatal("releasing over the hoop did not launch a scoring shot")
	}
}
