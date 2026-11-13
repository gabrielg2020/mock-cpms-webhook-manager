package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	testCases := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name: "uses defaults when nothing is set",
			env:  map[string]string{},
			want: Config{Environment: "development", ListenAddr: ":8080", DatabasePath: "data/app.db"},
		},
		{
			name: "reads values from the environment",
			env:  map[string]string{"APP_ENV": "production", "LISTEN_ADDR": ":9000", "DATABASE_PATH": "/data/app.db"},
			want: Config{Environment: "production", ListenAddr: ":9000", DatabasePath: "/data/app.db"},
		},
		{
			name:    "rejects an unknown environment",
			env:     map[string]string{"APP_ENV": "staging"},
			wantErr: true,
		},
		{
			name: "reads the seed settings",
			env:  map[string]string{"SEED_DEMO_DATA": "true", "SEED_ADMIN_PASSWORD": "admin-password-1", "SEED_MEMBER_PASSWORD": "member-password-1"},
			want: Config{
				Environment: "development", ListenAddr: ":8080", DatabasePath: "data/app.db",
				SeedDemoData: true, SeedAdminPassword: "admin-password-1", SeedMemberPassword: "member-password-1",
			},
		},
		{
			name: "does not need seed passwords when seeding is off",
			env:  map[string]string{"SEED_DEMO_DATA": "false"},
			want: Config{Environment: "development", ListenAddr: ":8080", DatabasePath: "data/app.db"},
		},
		{
			name:    "rejects a seed setting that is not true or false",
			env:     map[string]string{"SEED_DEMO_DATA": "yes"},
			wantErr: true,
		},
		{
			name:    "rejects seeding without passwords",
			env:     map[string]string{"SEED_DEMO_DATA": "true"},
			wantErr: true,
		},
		{
			name:    "rejects a short seed password",
			env:     map[string]string{"SEED_DEMO_DATA": "true", "SEED_ADMIN_PASSWORD": "short", "SEED_MEMBER_PASSWORD": "member-password-1"},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("APP_ENV", "")
			t.Setenv("LISTEN_ADDR", "")
			t.Setenv("DATABASE_PATH", "")
			t.Setenv("SEED_DEMO_DATA", "")
			t.Setenv("SEED_ADMIN_PASSWORD", "")
			t.Setenv("SEED_MEMBER_PASSWORD", "")
			for key, value := range testCase.env {
				t.Setenv(key, value)
			}

			got, err := Load()

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.want {
				t.Errorf("got %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestLoadNeverPutsAPasswordInAnErrorMessage(t *testing.T) {
	t.Setenv("SEED_DEMO_DATA", "true")
	t.Setenv("SEED_ADMIN_PASSWORD", "hunter2")
	t.Setenv("SEED_MEMBER_PASSWORD", "letmein")

	_, err := Load()

	if err == nil {
		t.Fatal("expected an error for short passwords, got nil")
	}
	for _, password := range []string{"hunter2", "letmein"} {
		if strings.Contains(err.Error(), password) {
			t.Errorf("error message contains the password %q: %v", password, err)
		}
	}
}
