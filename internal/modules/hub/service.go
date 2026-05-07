package hub

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"strings"
	"time"

	"modular-api/internal/modules/property"
	"modular-api/internal/platform/apperrors"
	"modular-api/internal/platform/storage"

	"gorm.io/gorm"
)

const defaultFeedLimit = 50

type Module struct {
	db      *gorm.DB
	storage storage.FileStorage
}

func NewModule(db *gorm.DB, fileStorage storage.FileStorage) *Module {
	return &Module{db: db, storage: fileStorage}
}

type ComposerAction struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
}

type Composer struct {
	Prompt    string           `json:"prompt"`
	PostLabel string           `json:"postLabel"`
	Actions   []ComposerAction `json:"actions"`
}

type FeedAuthor struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
	Meta      string `json:"meta"`
}

type ReplyItem struct {
	ID           string `json:"id"`
	AuthorName   string `json:"authorName"`
	AuthorAvatar string `json:"authorAvatarUrl,omitempty"`
	Content      string `json:"content"`
	Meta         string `json:"meta"`
}

type PostItem struct {
	ID           string      `json:"id"`
	Type         string      `json:"type"`
	Author       FeedAuthor  `json:"author"`
	Content      string      `json:"content"`
	ImageURL     string      `json:"imageUrl,omitempty"`
	LikeCount    int         `json:"likeCount"`
	CommentCount int         `json:"commentCount"`
	LikedByMe    bool        `json:"likedByMe"`
	Replies      []ReplyItem `json:"replies"`
	Event        *EventItem  `json:"event,omitempty"`
}

type EventItem struct {
	Title        string `json:"title"`
	Location     string `json:"location"`
	StartsAtISO  string `json:"startsAtIso"`
	StartsAtText string `json:"startsAtText"`
	EndsAtISO    string `json:"endsAtIso,omitempty"`
	EndsAtText   string `json:"endsAtText,omitempty"`
}

type Model struct {
	Header struct {
		Title string `json:"title"`
	} `json:"header"`
	Composer Composer   `json:"composer"`
	Feed     []PostItem `json:"feed"`
	Messages struct {
		Loading          string `json:"loading"`
		ErrorTitle       string `json:"errorTitle"`
		ErrorDescription string `json:"errorDescription"`
		EmptyComposer    string `json:"emptyComposer"`
	} `json:"messages"`
}

type ListInput struct {
	AccountCode string
	UnitCode    string
	Limit       int
}

type CreatePostInput struct {
	AccountCode   string
	UnitCode      string
	PostType      string
	Content       string
	AvatarURL     string
	ImageHeader   *multipart.FileHeader
	EventTitle    string
	EventDate     string
	EventEndDate  string
	EventLocation string
}

type CreateReplyInput struct {
	AccountCode string
	UnitCode    string
	Content     string
	AvatarURL   string
}

type ToggleLikeInput struct {
	AccountCode string
	UnitCode    string
}

type residentLookup struct {
	AccountCode  string
	ResidentCode string
	ResidentName string
	Email        string
	ResidentRole string
	Unit         property.Unit
	Area         property.Area
	Building     property.Building
}

func (m *Module) List(input ListInput) (*Model, error) {
	resident, err := m.lookupResidentAccount(strings.TrimSpace(input.AccountCode), strings.TrimSpace(input.UnitCode))
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 || limit > 100 {
		limit = defaultFeedLimit
	}

	var posts []Post
	if err := m.db.
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at asc")
		}).
		Where("building_code = ?", resident.Building.Code).
		Order("last_activity_at desc").
		Limit(limit).
		Find(&posts).Error; err != nil {
		return nil, apperrors.Internal("failed to load hub feed", err)
	}

	likedByPostID := map[uint]bool{}
	if len(posts) > 0 {
		postIDs := make([]uint, 0, len(posts))
		for _, post := range posts {
			postIDs = append(postIDs, post.ID)
		}

		var likes []PostLike
		if err := m.db.
			Where("account_code = ? AND post_id IN ?", resident.AccountCode, postIDs).
			Find(&likes).Error; err != nil {
			return nil, apperrors.Internal("failed to load hub likes", err)
		}

		for _, like := range likes {
			likedByPostID[like.PostID] = true
		}
	}

	model := defaultModel()
	model.Feed = make([]PostItem, 0, len(posts))
	for _, post := range posts {
		model.Feed = append(model.Feed, mapPost(post, likedByPostID[post.ID]))
	}

	return &model, nil
}

func (m *Module) CreatePost(ctx context.Context, input CreatePostInput) (*PostItem, error) {
	content := strings.TrimSpace(input.Content)
	postType := strings.TrimSpace(strings.ToLower(input.PostType))
	if postType == "" {
		postType = "post"
	}
	if postType != "post" && postType != "event" {
		return nil, apperrors.Validation("type must be post or event")
	}
	if postType == "post" && content == "" {
		return nil, apperrors.Validation("content is required")
	}

	resident, err := m.lookupResidentAccount(strings.TrimSpace(input.AccountCode), strings.TrimSpace(input.UnitCode))
	if err != nil {
		return nil, err
	}

	now := time.Now()
	post := Post{
		PublicID:       fmt.Sprintf("hub-post-%d", now.UnixMilli()),
		PostType:       postType,
		AccountCode:    resident.AccountCode,
		ResidentCode:   resident.ResidentCode,
		ResidentName:   resident.ResidentName,
		BuildingCode:   resident.Building.Code,
		BuildingName:   resident.Building.Name,
		UnitCode:       resident.Unit.Code,
		Content:        content,
		AuthorSnapshot: buildAuthorSnapshot(resident, input.AvatarURL),
		Media:          MediaAssets{},
		LastActivityAt: now,
	}

	if postType == "event" {
		eventDetails, err := buildEventDetails(input.EventTitle, input.EventLocation, input.EventDate, input.EventEndDate)
		if err != nil {
			return nil, err
		}

		post.EventDetails = eventDetails
		if content == "" {
			post.Content = fmt.Sprintf("%s at %s", eventDetails.Title, eventDetails.Location)
		}
	}

	if input.ImageHeader != nil {
		file, err := input.ImageHeader.Open()
		if err != nil {
			return nil, apperrors.Internal("failed to open hub image", err)
		}
		defer file.Close()

		storedFile, err := m.storage.Save(ctx, storage.SaveFileInput{
			Folder:      "hub/posts",
			FileName:    input.ImageHeader.Filename,
			ContentType: input.ImageHeader.Header.Get("Content-Type"),
			Reader:      file,
		})
		if err != nil {
			return nil, apperrors.Internal("failed to store hub image", err)
		}

		post.Media = MediaAssets{
			{
				Kind:            "image",
				StorageProvider: storedFile.StorageProvider,
				ObjectKey:       storedFile.ObjectKey,
				OriginalName:    storedFile.OriginalName,
				MimeType:        storedFile.MimeType,
				SizeBytes:       storedFile.SizeBytes,
				PublicURL:       storedFile.PublicURL,
			},
		}
	}

	if err := m.db.Create(&post).Error; err != nil {
		return nil, apperrors.Internal("failed to create hub post", err)
	}

	item := mapPost(post, false)
	return &item, nil
}

func (m *Module) CreateReply(input CreateReplyInput, postPublicID string) (*PostItem, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, apperrors.Validation("content is required")
	}

	resident, err := m.lookupResidentAccount(strings.TrimSpace(input.AccountCode), strings.TrimSpace(input.UnitCode))
	if err != nil {
		return nil, err
	}

	var item *PostItem
	err = m.db.Transaction(func(tx *gorm.DB) error {
		likedByMe := false
		var post Post
		if err := tx.Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at asc")
		}).Where("public_id = ?", postPublicID).First(&post).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NotFound("hub post not found")
			}
			return apperrors.Internal("failed to load hub post", err)
		}

		if post.BuildingCode != resident.Building.Code {
			return apperrors.NotFound("hub post not found")
		}

		var likeCount int64
		if err := tx.Model(&PostLike{}).
			Where("post_id = ? AND account_code = ?", post.ID, resident.AccountCode).
			Count(&likeCount).Error; err != nil {
			return apperrors.Internal("failed to load hub like state", err)
		}
		likedByMe = likeCount > 0

		reply := Reply{
			PublicID:       fmt.Sprintf("hub-reply-%d", time.Now().UnixNano()),
			PostID:         post.ID,
			AccountCode:    resident.AccountCode,
			ResidentCode:   resident.ResidentCode,
			ResidentName:   resident.ResidentName,
			BuildingCode:   resident.Building.Code,
			BuildingName:   resident.Building.Name,
			UnitCode:       resident.Unit.Code,
			Content:        content,
			AuthorSnapshot: buildAuthorSnapshot(resident, input.AvatarURL),
		}
		if err := tx.Create(&reply).Error; err != nil {
			return apperrors.Internal("failed to create hub reply", err)
		}

		if err := tx.Model(&post).Updates(map[string]any{
			"reply_count":      gorm.Expr("reply_count + ?", 1),
			"last_activity_at": reply.CreatedAt,
		}).Error; err != nil {
			return apperrors.Internal("failed to update hub post counters", err)
		}

		post.ReplyCount++
		post.LastActivityAt = reply.CreatedAt
		post.Replies = append(post.Replies, reply)
		mapped := mapPost(post, likedByMe)
		item = &mapped

		return nil
	})
	if err != nil {
		return nil, err
	}

	return item, nil
}

func (m *Module) ToggleLike(input ToggleLikeInput, postPublicID string) (*PostItem, error) {
	resident, err := m.lookupResidentAccount(strings.TrimSpace(input.AccountCode), strings.TrimSpace(input.UnitCode))
	if err != nil {
		return nil, err
	}

	var item *PostItem
	err = m.db.Transaction(func(tx *gorm.DB) error {
		var post Post
		if err := tx.Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at asc")
		}).Where("public_id = ?", postPublicID).First(&post).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NotFound("hub post not found")
			}
			return apperrors.Internal("failed to load hub post", err)
		}

		if post.BuildingCode != resident.Building.Code {
			return apperrors.NotFound("hub post not found")
		}

		likedByMe := false
		var existing PostLike
		switch err := tx.Where("post_id = ? AND account_code = ?", post.ID, resident.AccountCode).First(&existing).Error; {
		case err == nil:
			if err := tx.Delete(&existing).Error; err != nil {
				return apperrors.Internal("failed to remove hub like", err)
			}
			if err := tx.Model(&post).Update("like_count", gorm.Expr("GREATEST(like_count - 1, 0)")).Error; err != nil {
				return apperrors.Internal("failed to update hub like counter", err)
			}
			if post.LikeCount > 0 {
				post.LikeCount--
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			likedByMe = true
			like := PostLike{
				PostID:       post.ID,
				AccountCode:  resident.AccountCode,
				ResidentCode: resident.ResidentCode,
				BuildingCode: resident.Building.Code,
				UnitCode:     resident.Unit.Code,
			}
			if err := tx.Create(&like).Error; err != nil {
				return apperrors.Internal("failed to create hub like", err)
			}
			if err := tx.Model(&post).Update("like_count", gorm.Expr("like_count + ?", 1)).Error; err != nil {
				return apperrors.Internal("failed to update hub like counter", err)
			}
			post.LikeCount++
		default:
			return apperrors.Internal("failed to load hub like", err)
		}

		mapped := mapPost(post, likedByMe)
		item = &mapped
		return nil
	})
	if err != nil {
		return nil, err
	}

	return item, nil
}

func (m *Module) lookupResidentAccount(accountCode, unitCode string) (*residentLookup, error) {
	if accountCode == "" || unitCode == "" {
		return nil, apperrors.Validation("accountCode and unitCode are required")
	}

	accessProfile, err := property.ResolveAccessProfile(m.db, accountCode, unitCode)
	if err != nil {
		return nil, err
	}

	return &residentLookup{
		AccountCode:  accessProfile.AccountCode,
		ResidentCode: accessProfile.ResidentCode,
		ResidentName: accessProfile.ResidentName,
		Email:        accessProfile.Email,
		ResidentRole: accessProfile.ResidentRole,
		Unit:         accessProfile.Unit,
		Area:         accessProfile.Area,
		Building:     accessProfile.Building,
	}, nil
}

func buildAuthorSnapshot(resident *residentLookup, avatarURL string) AuthorSnapshot {
	return AuthorSnapshot{
		DisplayName:    resident.ResidentName,
		AvatarURL:      strings.TrimSpace(avatarURL),
		BuildingName:   resident.Building.Name,
		BuildingCode:   resident.Building.Code,
		UnitCode:       resident.Unit.Code,
		ResidentRole:   resident.ResidentRole,
		SecondaryLabel: resident.Email,
	}
}

func mapPost(post Post, likedByMe bool) PostItem {
	replies := make([]ReplyItem, 0, len(post.Replies))
	for _, reply := range post.Replies {
		replies = append(replies, ReplyItem{
			ID:           reply.PublicID,
			AuthorName:   reply.ResidentName,
			AuthorAvatar: reply.AuthorSnapshot.AvatarURL,
			Content:      reply.Content,
			Meta:         formatRelativeMeta(reply.CreatedAt, reply.BuildingName),
		})
	}

	return PostItem{
		ID:   post.PublicID,
		Type: post.PostType,
		Author: FeedAuthor{
			Name:      post.ResidentName,
			AvatarURL: post.AuthorSnapshot.AvatarURL,
			Meta:      formatRelativeMeta(post.CreatedAt, post.BuildingName),
		},
		Content:      post.Content,
		ImageURL:     firstImageURL(post.Media),
		LikeCount:    post.LikeCount,
		CommentCount: post.ReplyCount,
		LikedByMe:    likedByMe,
		Replies:      replies,
		Event:        mapEvent(post.EventDetails),
	}
}

func mapEvent(details *EventDetails) *EventItem {
	if details == nil {
		return nil
	}

	return &EventItem{
		Title:        details.Title,
		Location:     details.Location,
		StartsAtISO:  details.StartsAtISO,
		StartsAtText: details.StartsAtText,
		EndsAtISO:    details.EndsAtISO,
		EndsAtText:   details.EndsAtText,
	}
}

func firstImageURL(assets MediaAssets) string {
	for _, asset := range assets {
		if asset.Kind == "image" && asset.PublicURL != "" {
			return asset.PublicURL
		}
	}

	return ""
}

func defaultModel() Model {
	var model Model
	model.Header.Title = "Hub"
	model.Composer = Composer{
		Prompt:    "Share something with your neighbours",
		PostLabel: "Post",
		Actions: []ComposerAction{
			{
				ID:    "photo",
				Label: "Photo",
				Icon:  "image-outline",
			},
			{
				ID:    "event",
				Label: "Event",
				Icon:  "calendar-outline",
			},
		},
	}
	model.Messages.Loading = "Loading hub feed..."
	model.Messages.ErrorTitle = "Hub unavailable"
	model.Messages.ErrorDescription = "The community hub could not be loaded right now."
	model.Messages.EmptyComposer = "Write something before posting."

	return model
}

func formatRelativeMeta(createdAt time.Time, buildingName string) string {
	return fmt.Sprintf("%s • %s", relativeTime(createdAt), buildingName)
}

func relativeTime(value time.Time) string {
	if value.IsZero() {
		return "Just now"
	}

	elapsed := time.Since(value)
	switch {
	case elapsed < time.Minute:
		return "Just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(elapsed.Hours()))
	case elapsed < 48*time.Hour:
		return "Yesterday"
	case elapsed < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(elapsed.Hours()/24))
	default:
		return value.Format("02 Jan 2006")
	}
}

func residentRoleForUnit(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "tenanted":
		return "Tenant"
	case "owner occupied":
		return "Primary occupant"
	default:
		return "Resident"
	}
}

func buildEventDetails(title, location, startRaw, endRaw string) (*EventDetails, error) {
	trimmedTitle := strings.TrimSpace(title)
	trimmedLocation := strings.TrimSpace(location)
	trimmedStart := strings.TrimSpace(startRaw)
	trimmedEnd := strings.TrimSpace(endRaw)

	if trimmedTitle == "" || trimmedLocation == "" || trimmedStart == "" {
		return nil, apperrors.Validation("event title, location, and start date are required")
	}

	startsAt, err := time.Parse(time.RFC3339, trimmedStart)
	if err != nil {
		return nil, apperrors.Validation("eventDate must be a valid ISO datetime")
	}

	var endsAt time.Time
	if trimmedEnd != "" {
		endsAt, err = time.Parse(time.RFC3339, trimmedEnd)
		if err != nil {
			return nil, apperrors.Validation("eventEndDate must be a valid ISO datetime")
		}
		if endsAt.Before(startsAt) {
			return nil, apperrors.Validation("eventEndDate must be later than eventDate")
		}
	}

	details := &EventDetails{
		Title:        trimmedTitle,
		Location:     trimmedLocation,
		StartsAtISO:  startsAt.Format(time.RFC3339),
		StartsAtText: formatEventDateTime(startsAt),
	}

	if !endsAt.IsZero() {
		details.EndsAtISO = endsAt.Format(time.RFC3339)
		details.EndsAtText = formatEventDateTime(endsAt)
	}

	return details, nil
}

func formatEventDateTime(value time.Time) string {
	return value.Format("Mon, 02 Jan 2006 • 03:04 PM")
}
