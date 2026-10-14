package config

import (
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
			want: Config{Environment: "development", ListenAddr: ":8080"},
		},
		{
			name: "reads values from the environment",
			env:  map[string]string{"APP_ENV": "production", "LISTEN_ADDR": ":9000"},
			want: Config{Environment: "production", ListenAddr: ":9000"},
		},
		{
			name:    "rejects an unknown environment",
			env:     map[string]string{"APP_ENV": "staging"},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("APP_ENV", "")
			t.Setenv("LISTEN_ADDR", "")
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
