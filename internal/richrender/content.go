package richrender

// ContentFragment is one ordered fragment in the common rendering
// intermediate. Sources, notes, and chats adapt their native models to these
// fields before format-specific emission.
type ContentFragment struct {
	Kind          ContentFragmentKind
	Start         int
	End           int
	Text          string
	ImageURL      string
	ImageID       string
	ImageAlt      string
	ListMarker    string
	Language      string
	MemberName    string
	Bold          bool
	Italic        bool
	Code          bool
	RangeMismatch bool
	BlockStart    bool
}

// ContentFragmentKind identifies the structural role of a content fragment.
type ContentFragmentKind uint8

const (
	// ContentOrdinary is prose or other unclassified text.
	ContentOrdinary ContentFragmentKind = iota
	// ContentImage is an image reference.
	ContentImage
	// ContentCode is a code block.
	ContentCode
	// ContentMember is a member boundary in an archive-like source.
	ContentMember
)

// ContentModel supplies ordered fragments to RenderContent.
type ContentModel interface {
	ContentLen() int
	ContentFragment(int) ContentFragment
}

// ContentEmitter consumes classified fragments in reading order.
type ContentEmitter interface {
	EmitContent(ContentFragment) error
	FinishContent() error
}

// RenderContent classifies and emits content in one pass.
func RenderContent(model ContentModel, emitter ContentEmitter) error {
	for i := 0; i < model.ContentLen(); i++ {
		fragment := model.ContentFragment(i)
		fragment.Kind = ClassifyContentFragment(fragment)
		if err := emitter.EmitContent(fragment); err != nil {
			return err
		}
	}
	return emitter.FinishContent()
}

// ClassifyContentFragment returns the structural kind of fragment.
func ClassifyContentFragment(fragment ContentFragment) ContentFragmentKind {
	switch {
	case fragment.ImageURL != "":
		return ContentImage
	case fragment.MemberName != "":
		return ContentMember
	case fragment.Code:
		return ContentCode
	default:
		return ContentOrdinary
	}
}
