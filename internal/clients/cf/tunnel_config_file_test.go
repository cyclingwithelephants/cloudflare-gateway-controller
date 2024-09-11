package cf

import (
	"reflect"
	"testing"
)

func TestSort(t *testing.T) {
	tests := []struct {
		name   string
		routes []IngressConfig
		want   []IngressConfig
	}{
		{
			name: "Single fully qualified domain name",
			routes: []IngressConfig{
				{Hostname: "a.example.com", Service: "service1"},
			},
			want: []IngressConfig{
				{Hostname: "a.example.com", Service: "service1"},
			},
		},
		{
			name: "Fully qualified domain names sorted",
			routes: []IngressConfig{
				{Hostname: "b.example.com", Service: "service2"},
				{Hostname: "a.example.com", Service: "service1"},
			},
			want: []IngressConfig{
				{Hostname: "a.example.com", Service: "service1"},
				{Hostname: "b.example.com", Service: "service2"},
			},
		},
		{
			name: "Wildcard routes sorted after FQDN",
			routes: []IngressConfig{
				{Hostname: "b.example.com", Service: "service2"},
				{Hostname: "*.example.com", Service: "service3"},
				{Hostname: "a.example.com", Service: "service1"},
			},
			want: []IngressConfig{
				{Hostname: "a.example.com", Service: "service1"},
				{Hostname: "b.example.com", Service: "service2"},
				{Hostname: "*.example.com", Service: "service3"},
			},
		},
		{
			name: "Catch-all route at the end",
			routes: []IngressConfig{
				{Hostname: "b.example.com", Service: "service2"},
				{Hostname: "", Service: "service4"}, // catch-all route
				{Hostname: "*.example.com", Service: "service3"},
				{Hostname: "a.example.com", Service: "service1"},
			},
			want: []IngressConfig{
				{Hostname: "a.example.com", Service: "service1"},
				{Hostname: "b.example.com", Service: "service2"},
				{Hostname: "*.example.com", Service: "service3"},
				{Hostname: "", Service: "service4"}, // catch-all at the end
			},
		},
		{
			name: "Only catch-all route",
			routes: []IngressConfig{
				{Hostname: "", Service: "catch-all"},
			},
			want: []IngressConfig{
				{Hostname: "", Service: "catch-all"},
			},
		},
		{
			name:   "No routes",
			routes: []IngressConfig{},
			want:   []IngressConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Sort(tt.routes)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Sort() = %v, want %v", got, tt.want)
			}
		})
	}
}
