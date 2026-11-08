package main

import "testing"

func TestSplitDNSName(t *testing.T) {
	tests := []struct {
		name       string
		fqdn       string
		zone       string
		wantZone   string
		wantRecord string
	}{
		{
			name:       "subdomain with trailing dots",
			fqdn:       "_acme-challenge.www.example.com.",
			zone:       "example.com.",
			wantZone:   "example.com",
			wantRecord: "_acme-challenge.www",
		},
		{
			name:       "subdomain without trailing dots",
			fqdn:       "_acme-challenge.example.com",
			zone:       "example.com",
			wantZone:   "example.com",
			wantRecord: "_acme-challenge",
		},
		{
			name:       "apex domain (FQDN equals zone)",
			fqdn:       "example.com.",
			zone:       "example.com.",
			wantZone:   "example.com",
			wantRecord: "@",
		},
		{
			name:       "deep subdomain",
			fqdn:       "_acme-challenge.a.b.c.example.com.",
			zone:       "example.com.",
			wantZone:   "example.com",
			wantRecord: "_acme-challenge.a.b.c",
		},
		{
			name:       "mixed trailing dots — fqdn has dot, zone does not",
			fqdn:       "_acme-challenge.example.com.",
			zone:       "example.com",
			wantZone:   "example.com",
			wantRecord: "_acme-challenge",
		},
		{
			name:       "mixed trailing dots — fqdn no dot, zone has dot",
			fqdn:       "_acme-challenge.example.com",
			zone:       "example.com.",
			wantZone:   "example.com",
			wantRecord: "_acme-challenge",
		},
		{
			name:       "single-label challenge label",
			fqdn:       "_acme-challenge.example.com.",
			zone:       "example.com.",
			wantZone:   "example.com",
			wantRecord: "_acme-challenge",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotZone, gotRecord := splitDNSName(tc.fqdn, tc.zone)
			if gotZone != tc.wantZone {
				t.Errorf("zone: got %q, want %q", gotZone, tc.wantZone)
			}
			if gotRecord != tc.wantRecord {
				t.Errorf("record name: got %q, want %q", gotRecord, tc.wantRecord)
			}
		})
	}
}
