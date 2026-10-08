package templates

import "github.com/koalastuff/koalabye/internal/db"

// ResponsesPageData is the view model for the campaign responses list.
type ResponsesPageData struct {
	Page          db.ResponsePage
	Filter        db.ResponseFilter
	Tags          []db.ResponseTag
	Views         []db.ResponseView
	Members       []db.AssigneeOption
	CanEdit       bool
	IsOwner       bool
	AutoCloseDays int64
}

// ResponseDetailData is the view model for a single response.
type ResponseDetailData struct {
	Submission    db.Submission
	Readers       []db.SubmissionReader
	Neighbors     db.SubmissionNeighbors
	Notes         []db.SubmissionNote
	Tags          []db.ResponseTag
	Members       []db.AssigneeOption
	CanEdit       bool
	CurrentUserID int64
}
