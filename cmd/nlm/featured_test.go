package main

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestFeaturedProjectDescriptionCollapsesWhitespace(t *testing.T) {
	project := &pb.FeaturedProject{
		Presentation: &pb.FeaturedProjectPresentation{
			Description: "Line one\n\n- bullet\titem",
		},
	}
	got := featuredProjectDescription(project)
	want := "Line one - bullet item"
	if got != want {
		t.Fatalf("featuredProjectDescription() = %q, want %q", got, want)
	}
}

func TestFeaturedProjectDescriptionFallsBackToSourceCount(t *testing.T) {
	project := &pb.FeaturedProject{
		Sources: []*pb.Source{{}, {}},
	}
	got := featuredProjectDescription(project)
	if got != "2 sources" {
		t.Fatalf("featuredProjectDescription() = %q, want %q", got, "2 sources")
	}
}
