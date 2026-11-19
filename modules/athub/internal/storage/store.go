package storage

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("record not found")
	ErrInvalidReference = errors.New("platform, author, novel, volume or tag reference is invalid")
	ErrConflict         = errors.New("record already exists")
	ErrVolumeNotEmpty   = errors.New("volume still contains chapters")
	ErrTagInUse         = errors.New("tag is still referenced by novels")
	ErrImageMissing     = errors.New("image is no longer stored on the image host")
	ErrStale            = errors.New("content changed after the change was submitted")
)

type PlatformInput struct {
	Name string  `json:"name"`
	URL  *string `json:"url"`
}

type Platform struct {
	ID int32 `json:"id"`
	PlatformInput
}

type AuthorInput struct {
	PlatformID int32   `json:"platformId"`
	Name       *string `json:"name"`
}

type Author struct {
	ID int32 `json:"id"`
	AuthorInput
}

type NovelInput struct {
	AuthorName *string  `json:"authorName,omitempty"`
	TagNames   []string `json:"tagNames,omitempty"`
	TagIDs     []int32  `json:"tagIds"`
	PlatformID int32    `json:"platformId"`
	AuthorID   *int32   `json:"authorId"`
	Title      string   `json:"title"`
	Summary    *string  `json:"summary"`
	CoverURL   *string  `json:"coverUrl"`
	Status     string   `json:"status"`
}

type Novel struct {
	ID int32 `json:"id"`
	NovelInput
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ChapterInput struct {
	NovelID  int32  `json:"novelId"`
	VolumeID *int32 `json:"volumeId"`
	Order    int32  `json:"order"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

type Chapter struct {
	ID int32 `json:"id"`
	ChapterInput
	UpdatedAt time.Time `json:"updatedAt"`
}

type ChapterSummary struct {
	ID        int32     `json:"id"`
	NovelID   int32     `json:"novelId"`
	VolumeID  *int32    `json:"volumeId"`
	Order     int32     `json:"order"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type TOC struct {
	NovelID   int32            `json:"novelId"`
	WordCount int64            `json:"wordCount"`
	Volumes   []Volume         `json:"volumes"`
	Chapters  []ChapterSummary `json:"chapters"`
}

type VolumeInput struct {
	NovelID int32  `json:"novelId"`
	Order   int32  `json:"order"`
	Title   string `json:"title"`
}

type Volume struct {
	ID int32 `json:"id"`
	VolumeInput
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type UploadVolume struct {
	Order int32  `json:"order"`
	Title string `json:"title"`
}

type UploadChapter struct {
	Order       int32  `json:"order"`
	VolumeOrder *int32 `json:"volumeOrder"`
	Title       string `json:"title"`
	Content     string `json:"content"`
}

type UploadInput struct {
	AuthorName *string         `json:"authorName,omitempty"`
	NovelID    *int32          `json:"novelId"`
	Novel      *NovelInput     `json:"novel"`
	Volumes    []UploadVolume  `json:"volumes"`
	Chapters   []UploadChapter `json:"chapters"`
}

type UploadedChapter struct {
	ID        int32     `json:"id"`
	Order     int32     `json:"order"`
	VolumeID  *int32    `json:"volumeId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type UploadResult struct {
	Novel        Novel             `json:"novel"`
	Volumes      []Volume          `json:"volumes"`
	Chapters     []UploadedChapter `json:"chapters"`
	ChapterCount int               `json:"chapterCount"`
}

type NovelFilter struct {
	PlatformID int32
	AuthorID   int32
	TagID      int32
	Limit      int32
	Offset     int32
}

type Store interface {
	GetTag(context.Context, int32) (Tag, error)
	ListTags(context.Context, int32, int32) ([]Tag, error)
	CreateTag(context.Context, TagInput) (Tag, error)
	UpdateTag(context.Context, int32, TagInput) (Tag, error)
	DeleteTag(context.Context, int32) error
	GetPlatform(context.Context, int32) (Platform, error)
	ListPlatforms(context.Context, int32, int32) ([]Platform, error)
	CreatePlatform(context.Context, PlatformInput) (Platform, error)
	GetAuthor(context.Context, int32) (Author, error)
	ListAuthors(context.Context, int32, int32, int32) ([]Author, error)
	CreateAuthor(context.Context, AuthorInput) (Author, error)
	UpdateAuthorName(context.Context, int32, *string) (Author, error)
	GetNovel(context.Context, int32) (Novel, error)
	ListNovels(context.Context, NovelFilter) ([]Novel, error)
	CreateNovel(context.Context, NovelInput) (Novel, error)
	UpdateNovel(context.Context, int32, NovelInput) (Novel, error)
	DeleteNovel(context.Context, int32) error
	GetTOC(context.Context, int32) (TOC, error)
	GetChapter(context.Context, int32, int32) (Chapter, error)
	ListChapters(context.Context, int32, int32, int32) ([]Chapter, error)
	SaveChapter(context.Context, ChapterInput) (Chapter, error)
	DeleteChapter(context.Context, int32, int32) error
	GetVolume(context.Context, int32, int32) (Volume, error)
	ListVolumes(context.Context, int32, int32, int32) ([]Volume, error)
	SaveVolume(context.Context, VolumeInput) (Volume, error)
	DeleteVolume(context.Context, int32, int32) error
	Upload(context.Context, UploadInput) (UploadResult, error)
}
