package user

import (
	"testing"
)

func TestContactPasswordLogin(t *testing.T) {
	m, db := newTestManager(t)

	var id int
	if err := db.Get(&id, `
		INSERT INTO users (type, email, first_name, last_name)
		VALUES ('contact', 'customer@example.com', 'Ivan', 'Petrov')
		RETURNING id`); err != nil {
		t.Fatal(err)
	}

	if err := m.SetContactPassword(id, "Client-2026!Pass"); err != nil {
		t.Fatalf("setting contact password: %v", err)
	}

	user, err := m.VerifyContactPassword("CUSTOMER@example.com", []byte("Client-2026!Pass"))
	if err != nil || user.ID != id {
		t.Fatalf("contact login: user=%+v err=%v", user, err)
	}
	if _, err := m.VerifyContactPassword("customer@example.com", []byte("wrong-password")); err == nil {
		t.Fatal("wrong contact password accepted")
	}

	if err := m.SetContactPassword(id, "short"); err == nil {
		t.Fatal("weak contact password accepted")
	}

	if _, err := db.Exec(`UPDATE users SET enabled = false WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifyContactPassword("customer@example.com", []byte("Client-2026!Pass")); err == nil {
		t.Fatal("disabled contact accepted")
	}
}

func TestContactPasswordRejectsAgents(t *testing.T) {
	m, db := newTestManager(t)

	var id int
	if err := db.Get(&id, `
		INSERT INTO users (type, email, first_name, last_name)
		VALUES ('agent', 'agent@example.com', 'Agent', '')
		RETURNING id`); err != nil {
		t.Fatal(err)
	}
	if err := m.SetContactPassword(id, "Agent-2026!Pass"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifyContactPassword("agent@example.com", []byte("Agent-2026!Pass")); err == nil {
		t.Fatal("agent accepted by the contact password login")
	}
}
