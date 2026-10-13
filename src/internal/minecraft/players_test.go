package minecraft

import "testing"

func TestParsePlayerCount(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     int
		wantErr  bool
	}{
		{name: "empty server", response: "There are 0 of a max of 20 players online:", want: 0},
		{name: "occupied server", response: "There are 2 of a max of 20 players online: Alex, Steve", want: 2},
		{name: "invalid response", response: "Unknown command", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParsePlayerCount(test.response)
			if (err != nil) != test.wantErr {
				t.Fatalf("ParsePlayerCount() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Errorf("ParsePlayerCount() = %d, want %d", got, test.want)
			}
		})
	}
}
