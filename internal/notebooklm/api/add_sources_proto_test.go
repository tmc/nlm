package api

import (
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

type addSourceIdentity struct{ ID, Title string }

func sourceID(source *pb.Source) string {
	if source == nil || source.GetSourceId() == nil {
		return ""
	}
	return source.GetSourceId().GetSourceId()
}

func sourceTitle(source *pb.Source) string {
	if source == nil {
		return ""
	}
	return source.GetTitle()
}
