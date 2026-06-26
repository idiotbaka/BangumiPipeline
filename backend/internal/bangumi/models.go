package bangumi

import "encoding/json"

const (
	imageStatusPending    = "pending"
	imageStatusDownloaded = "downloaded"
	imageStatusFailed     = "failed"
	imageStatusNotFound   = "not_found"
)

type imageDownload struct {
	Path   string
	Status string
}

type calendarDay struct {
	Items []calendarItem `json:"items"`
}

type calendarItem struct {
	ID         int64    `json:"id"`
	URL        string   `json:"url"`
	Type       int      `json:"type"`
	Name       string   `json:"name"`
	NameCN     string   `json:"name_cn"`
	AirDate    string   `json:"air_date"`
	AirWeekday int      `json:"air_weekday"`
	Images     imageSet `json:"images"`
}

type subjectDetail struct {
	Date          string          `json:"date"`
	Platform      string          `json:"platform"`
	Images        imageSet        `json:"images"`
	Summary       string          `json:"summary"`
	Name          string          `json:"name"`
	NameCN        string          `json:"name_cn"`
	Tags          []subjectTag    `json:"tags"`
	Infobox       []infoboxItem   `json:"infobox"`
	Rating        json.RawMessage `json:"rating"`
	TotalEpisodes int             `json:"total_episodes"`
	Collection    json.RawMessage `json:"collection"`
	ID            int64           `json:"id"`
	Eps           int             `json:"eps"`
	MetaTags      []string        `json:"meta_tags"`
	Volumes       int             `json:"volumes"`
	Series        bool            `json:"series"`
	Locked        bool            `json:"locked"`
	NSFW          bool            `json:"nsfw"`
	Type          int             `json:"type"`
}

type subjectTag struct {
	Name       string `json:"name"`
	Count      int    `json:"count"`
	TotalCount int    `json:"total_cont"`
}

type infoboxItem struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type relatedCharacter struct {
	Images   imageSet       `json:"images"`
	Name     string         `json:"name"`
	Summary  string         `json:"summary"`
	Relation string         `json:"relation"`
	Actors   []relatedActor `json:"actors"`
	Type     int            `json:"type"`
	ID       int64          `json:"id"`
}

type relatedActor struct {
	Images       imageSet `json:"images"`
	Name         string   `json:"name"`
	ShortSummary string   `json:"short_summary"`
	Career       []string `json:"career"`
	ID           int64    `json:"id"`
	Type         int      `json:"type"`
	Locked       bool     `json:"locked"`
}

type imageSet struct {
	Large string `json:"large"`
}

type incompleteSubject struct {
	BangumiID       int64
	DetailStatus    string
	CharacterStatus string
	EpisodesStatus  string
	EpisodesMissing bool
}

type storedCharacter struct {
	CharacterID   int64
	Name          string
	Summary       string
	Relation      string
	Type          int
	ImageLargeURL string
	ImagePath     string
	ImageStatus   string
	ImageError    string
	ActorIDs      []int64
}

type storedActor struct {
	ActorID       int64
	Name          string
	ShortSummary  string
	CareerJSON    string
	Type          int
	Locked        bool
	ImageLargeURL string
	ImagePath     string
	ImageStatus   string
	ImageError    string
}

type actorImageState struct {
	Exists        bool
	ImageLargeURL string
	ImagePath     string
	ImageStatus   string
}

type pendingAnimeImage struct {
	BangumiID int64
	SourceURL string
}

type episodesResponse struct {
	Data   []episodeDetail `json:"data"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

type episodeDetail struct {
	Airdate         string  `json:"airdate"`
	Name            string  `json:"name"`
	NameCN          string  `json:"name_cn"`
	Duration        string  `json:"duration"`
	Description     string  `json:"desc"`
	Ep              int     `json:"ep"`
	Sort            float64 `json:"sort"`
	ID              int64   `json:"id"`
	SubjectID       int64   `json:"subject_id"`
	Comment         int     `json:"comment"`
	Type            int     `json:"type"`
	Disc            int     `json:"disc"`
	DurationSeconds int     `json:"duration_seconds"`
}
