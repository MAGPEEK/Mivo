package playback_test

import (
	"testing"
	"testing/synctest"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
)

var _ appplayback.CPUReporter = (*fakeProduction)(nil)

func TestListCarriesReasonsClientClassAndCPU(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, env *environment) {
		service := env.service(t)

		remote := request(transcodeFileID)
		// Local stays false: the zero value is remote.

		transcode, err := service.Start(adminContext(t), remote)
		if err != nil {
			t.Fatalf("Start error = %v", err)
		}

		localRequest := request(directFileID)
		localRequest.Local = true
		direct := mustStart(t, service, localRequest)

		synctest.Wait()
		env.producer.productionOf(transcode.ID).setCPUPercent(183.5)

		got := listed(t, service, transcode.ID)
		if got.Local || len(got.Reasons) == 0 || got.CPUPercent != 183.5 {
			t.Fatalf("transcode info = %+v, want remote with reasons and 183.5 %% CPU", got)
		}

		if got.Reasons[0] != transcode.Plan.Reasons[0] {
			t.Errorf("reasons = %v, want the plan's %v", got.Reasons, transcode.Plan.Reasons)
		}

		local := listed(t, service, direct.ID)
		if !local.Local || len(local.Reasons) != 0 || local.CPUPercent != 0 {
			t.Errorf("direct info = %+v, want local, no reasons, no CPU", local)
		}

		// A caller must not be able to change the stored plan through the listing.
		got.Reasons[0].Code = "changed"

		if again := listed(t, service, transcode.ID); again.Reasons[0].Code == "changed" {
			t.Error("List shares the reasons slice with the registry")
		}
	})
}
