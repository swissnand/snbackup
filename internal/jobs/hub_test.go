package jobs

import (
	"testing"
	"time"

	"github.com/swissnand/snbackup/internal/store"
)

func TestDispatchPollReport(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Update(func(s *store.State) error {
		s.Agents = append(s.Agents, &store.Agent{ID: "ag1", Name: "web-01"})
		s.Repos = append(s.Repos, &store.Repository{ID: "r1", Name: "NAS", Password: st.Box().Encrypt("geheim-passwort")})
		s.Jobs = append(s.Jobs, &store.Job{ID: "j1", Name: "Job", Type: "backup", AgentID: "ag1", RepoID: "r1",
			Source: store.Source{DBPassword: st.Box().Encrypt("db-pw")}, Notify: store.Notify{OnFailure: true, Emails: []string{"x@y.ch"}}})
		return nil
	})
	m := New(st)
	if err := m.Trigger("j1", "manual", false); err != nil {
		t.Fatal(err)
	}
	if !m.IsBusy("j1") {
		t.Fatal("job should be busy")
	}
	tasks, _ := m.AgentPoll("ag1")
	if len(tasks) != 1 || tasks[0].Repos[0].Password != "geheim-passwort" || tasks[0].Job.Source.DBPassword != "db-pw" || len(tasks[0].Job.Notify.Emails) != 0 {
		t.Fatalf("unexpected task %+v", tasks)
	}
	if !m.AgentOnline("ag1") {
		t.Fatal("agent should be online")
	}
	if other, _ := m.AgentPoll("ag2"); len(other) != 0 {
		t.Fatal("task leaked to other agent")
	}
	if err := m.AgentReport("ag2", AgentReport{RunID: tasks[0].RunID}); err == nil {
		t.Fatal("foreign agent accepted")
	}
	_ = m.AgentReport("ag1", AgentReport{RunID: tasks[0].RunID, Phase: "Daten sichern", Log: []string{"zeile"}})
	if l := m.Running(); len(l) != 1 || l[0].Phase != "Daten sichern" {
		t.Fatalf("live not updated: %+v", l)
	}
	_ = m.AgentReport("ag1", AgentReport{RunID: tasks[0].RunID, Done: true, Status: "success", Message: "ok"})
	time.Sleep(50 * time.Millisecond)
	if m.IsBusy("j1") {
		t.Fatal("job still busy")
	}
	runs := st.Runs("j1", 1)
	if len(runs) != 1 || runs[0].Status != "success" || runs[0].Agent != "web-01" {
		t.Fatalf("run %+v", runs)
	}
	if j, _ := st.Job("j1"); j.LastStatus != "success" {
		t.Fatal("job status not updated")
	}
}
