package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/ownership"
)

func (s *authStack) roleOf(t *testing.T, bearer string) string {
	t.Helper()
	var me struct{ Role string }
	json.Unmarshal(s.req("GET", "/auth/me", bearer, "").Body.Bytes(), &me)
	return me.Role
}

func (s *authStack) startTransfer(bearer, target, role, password string) int {
	return s.req("POST", "/ownership/transfer", bearer, fmt.Sprintf(`{"targetId":%q,"formerRole":%q,"password":%q}`, target, role, password)).Code
}

func TestOwnershipTransfer_TwoStepsWithPasswordsAndNothingChangesUntilAccepted(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "senha-do-dono")
	s.addUserWithPassword(t, "adm", "admin", "s3cret")
	anaID := s.addUserWithPassword(t, "ana", "reader", "senha-da-ana")
	s.addUserWithPassword(t, "bob", "reader", "s3cret")
	boss, adm, ana, bob := s.login(t, "boss", "senha-do-dono"), s.login(t, "adm", "s3cret"), s.login(t, "ana", "senha-da-ana"), s.login(t, "bob", "s3cret")

	// Only the owner starts, with their password and an explicit choice for their own role.
	if code := s.startTransfer(adm, anaID, "reader", "s3cret"); code != http.StatusForbidden {
		t.Errorf("an admin starts: %d, want 403", code)
	}
	if code := s.startTransfer(boss, anaID, "reader", "errada"); code != http.StatusForbidden {
		t.Errorf("wrong password: %d, want 403", code)
	}
	if code := s.startTransfer(boss, anaID, "", "senha-do-dono"); code != http.StatusBadRequest {
		t.Errorf("no former role: %d, want 400 (there is no default)", code)
	}
	if code := s.startTransfer(boss, "not-a-uuid", "reader", "senha-do-dono"); code != http.StatusBadRequest {
		t.Errorf("malformed target: %d, want 400", code)
	}
	if s.scalar(t, `SELECT count(*) FROM ownership_transfers`) != "0" {
		t.Fatal("a refused start left an offer")
	}

	if code := s.startTransfer(boss, anaID, "admin", "senha-do-dono"); code != http.StatusCreated {
		t.Fatalf("start: %d", code)
	}
	if code := s.startTransfer(boss, anaID, "admin", "senha-do-dono"); code != http.StatusConflict {
		t.Errorf("a second offer: %d, want 409", code)
	}
	// Nobody's role moved.
	if s.roleOf(t, boss) != "owner" || s.roleOf(t, ana) != "reader" {
		t.Fatal("roles changed before acceptance")
	}

	view := func(token string) (outgoing bool, from, to string, has bool) {
		var out struct {
			Transfer *struct{ From, To, FormerRole string }
			Outgoing bool
		}
		json.Unmarshal(s.req("GET", "/ownership/transfer", token, "").Body.Bytes(), &out)
		if out.Transfer == nil {
			return false, "", "", false
		}
		return out.Outgoing, out.Transfer.From, out.Transfer.To, true
	}
	if out, from, to, has := view(boss); !has || !out || from != "boss" || to != "ana" {
		t.Errorf("the owner's view: %v %s %s %v", out, from, to, has)
	}
	if out, _, _, has := view(ana); !has || out {
		t.Errorf("the target's view: outgoing=%v has=%v", out, has)
	}
	if _, _, _, has := view(bob); has {
		t.Error("an uninvolved account sees the offer")
	}

	// Accepting means signing in again, and only the target can.
	if code := s.req("POST", "/ownership/transfer/accept", ana, `{"password":"errada"}`).Code; code != http.StatusForbidden {
		t.Errorf("accept with a wrong password: %d, want 403", code)
	}
	if code := s.req("POST", "/ownership/transfer/accept", bob, `{"password":"s3cret"}`).Code; code != http.StatusNotFound {
		t.Errorf("accept by someone else: %d, want 404", code)
	}
	if code := s.req("POST", "/ownership/transfer/accept", boss, `{"password":"senha-do-dono"}`).Code; code != http.StatusNotFound {
		t.Errorf("the owner accepting their own offer: %d, want 404", code)
	}
	if s.roleOf(t, boss) != "owner" {
		t.Fatal("a refused acceptance moved the ownership")
	}
	if code := s.req("POST", "/ownership/transfer/accept", ana, `{"password":"senha-da-ana"}`); code.Code != http.StatusNoContent {
		t.Fatalf("accept: %d", code.Code)
	}
	if s.roleOf(t, ana) != "owner" || s.roleOf(t, boss) != "admin" {
		t.Errorf("roles after: ana=%s boss=%s", s.roleOf(t, ana), s.roleOf(t, boss))
	}
	// The former owner no longer starts transfers; the new one does.
	if code := s.startTransfer(boss, anaID, "reader", "senha-do-dono"); code != http.StatusForbidden {
		t.Errorf("the former owner starts another: %d, want 403", code)
	}
}

func TestOwnershipTransfer_CancelAndDecline(t *testing.T) {
	s := newAuthStack(t)
	s.addUserWithPassword(t, "boss", "owner", "s3cret")
	anaID := s.addUserWithPassword(t, "ana", "reader", "s3cret")
	boss, ana := s.login(t, "boss", "s3cret"), s.login(t, "ana", "s3cret")

	s.startTransfer(boss, anaID, "reader", "s3cret")
	if code := s.req("DELETE", "/ownership/transfer", ana, "").Code; code != http.StatusNotFound {
		t.Errorf("the target cancelling: %d, want 404", code)
	}
	if code := s.req("DELETE", "/ownership/transfer", boss, "").Code; code != http.StatusNoContent {
		t.Errorf("cancel: %d", code)
	}
	if code := s.req("POST", "/ownership/transfer/accept", ana, `{"password":"s3cret"}`).Code; code != http.StatusNotFound {
		t.Errorf("accepting a cancelled offer: %d, want 404", code)
	}

	s.startTransfer(boss, anaID, "reader", "s3cret")
	if code := s.req("POST", "/ownership/transfer/decline", ana, ""); code.Code != http.StatusNoContent {
		t.Errorf("decline: %d", code.Code)
	}
	if s.roleOf(t, boss) != "owner" || s.roleOf(t, ana) != "reader" {
		t.Error("roles changed")
	}
}

func TestRecovery_TheOwnerIsToldAtTheNextSignInAndTheLinkResetsThePassword(t *testing.T) {
	s := newAuthStack(t)
	bossID := s.addUserWithPassword(t, "boss", "owner", "esquecida")
	s.addUserWithPassword(t, "ana", "reader", "s3cret")
	old := s.login(t, "boss", "esquecida")
	ana := s.login(t, "ana", "s3cret")

	// What the command on the server does; there is no HTTP route for it.
	token, _, name, err := ownership.RecoverAccess(context.Background(), s.db)
	if err != nil || name != "boss" {
		t.Fatal(err)
	}
	if rec := s.req("GET", "/probe", old, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("the owner's old session after recovery: %d, want 401", rec.Code)
	}
	if code := s.useReset(token, "senha-nova-1"); code != http.StatusNoContent {
		t.Fatalf("the printed link: %d", code)
	}
	if code := s.useReset(token, "outra-senha-9"); code != http.StatusNotFound {
		t.Errorf("using it twice: %d, want 404", code)
	}

	fresh := s.login(t, "boss", "senha-nova-1")
	var me struct {
		Notices []struct {
			ID   int64
			Kind string
		}
	}
	json.Unmarshal(s.req("GET", "/auth/me", fresh, "").Body.Bytes(), &me)
	if len(me.Notices) != 1 || me.Notices[0].Kind != "owner_recovery_reset" {
		t.Fatalf("notices = %+v", me.Notices)
	}
	// Others see nothing of it, and cannot dismiss it.
	if strings.Contains(s.req("GET", "/auth/me", ana, "").Body.String(), "owner_recovery") {
		t.Error("another account is shown the owner's notice")
	}
	ack := fmt.Sprintf("/auth/notices/%d/ack", me.Notices[0].ID)
	if code := s.req("POST", ack, ana, "").Code; code != http.StatusNotFound {
		t.Errorf("another account dismissing it: %d, want 404", code)
	}
	if code := s.req("POST", ack, fresh, "").Code; code != http.StatusNoContent {
		t.Errorf("dismiss: %d", code)
	}
	json.Unmarshal(s.req("GET", "/auth/me", fresh, "").Body.Bytes(), &me)
	if len(me.Notices) != 0 {
		t.Error("the notice came back after being dismissed")
	}
	if code := s.req("POST", ack, fresh, "").Code; code != http.StatusNotFound {
		t.Errorf("dismissing twice: %d, want 404", code)
	}
	_ = bossID
}
