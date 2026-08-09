package updatecheck

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"icloud-privacy-mail-v2/internal/buildinfo"
)

const (
	cacheTTL        = 10 * time.Minute
	requestTimeout  = 20 * time.Second
	responseMaxSize = 4 << 20
)

//go:embed announcements.json
var announcementFiles embed.FS

type Announcement struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Content     string `json:"content"`
	PublishedAt string `json:"published_at"`
	URL         string `json:"url,omitempty"`
}

type LatestInfo struct {
	Version     string `json:"version"`
	Name        string `json:"name"`
	Notes       string `json:"notes"`
	PublishedAt string `json:"published_at"`
	URL         string `json:"url"`
	Source      string `json:"source"`
	Commit      string `json:"commit,omitempty"`
}

type Status struct {
	Enabled           bool           `json:"enabled"`
	Repository        string         `json:"repository"`
	RepositoryURL     string         `json:"repository_url"`
	Current           buildinfo.Info `json:"current"`
	Latest            *LatestInfo    `json:"latest,omitempty"`
	UpdateAvailable   bool           `json:"update_available"`
	CheckedAt         string         `json:"checked_at"`
	Error             string         `json:"error,omitempty"`
	AnnouncementError string         `json:"announcement_error,omitempty"`
	Announcements     []Announcement `json:"announcements"`
}

type Service struct {
	enabled    bool
	repository string
	client     *http.Client
	mu         sync.Mutex
	cachedAt   time.Time
	cached     Status
}

type httpStatusError struct {
	StatusCode int
	Body       string
}

func (e *httpStatusError) Error() string {
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d：%s", e.StatusCode, strings.TrimSpace(e.Body))
}

type announcementDocument struct {
	Announcements []Announcement `json:"announcements"`
}

func New(enabled bool, repository string) *Service {
	return &Service{
		enabled:    enabled,
		repository: strings.Trim(strings.TrimSpace(repository), "/"),
		client:     &http.Client{Timeout: requestTimeout},
	}
}

// Check 检查 GitHub Release、默认分支提交和项目公告。
func (s *Service) Check(ctx context.Context, force bool) Status {
	now := time.Now()
	s.mu.Lock()
	if !force && !s.cachedAt.IsZero() && now.Sub(s.cachedAt) < cacheTTL {
		cached := s.cached
		s.mu.Unlock()
		return cached
	}
	s.mu.Unlock()

	status := s.check(ctx, now)
	s.mu.Lock()
	s.cached = status
	s.cachedAt = now
	s.mu.Unlock()
	return status
}

func (s *Service) check(ctx context.Context, checkedAt time.Time) Status {
	repositoryURL := ""
	if validRepository(s.repository) {
		repositoryURL = "https://github.com/" + s.repository
	}
	status := Status{
		Enabled:       s.enabled,
		Repository:    s.repository,
		RepositoryURL: repositoryURL,
		Current:       buildinfo.Current(),
		CheckedAt:     checkedAt.Format(time.RFC3339),
		Announcements: builtinAnnouncements(),
	}
	if !s.enabled {
		return status
	}
	if !validRepository(s.repository) {
		status.Error = "更新仓库格式不正确，应为 owner/repository"
		return status
	}

	if announcements, err := s.fetchRepositoryAnnouncements(ctx); err != nil {
		status.AnnouncementError = "读取项目公告失败：" + err.Error()
	} else {
		status.Announcements = mergeAnnouncements(status.Announcements, announcements)
	}

	latest, updateAvailable, releaseAnnouncement, err := s.fetchLatest(ctx, status.Current)
	if err != nil {
		status.Error = "检查更新失败：" + err.Error()
		return status
	}
	status.Latest = latest
	status.UpdateAvailable = updateAvailable
	if releaseAnnouncement != nil {
		status.Announcements = mergeAnnouncements(status.Announcements, []Announcement{*releaseAnnouncement})
	}
	return status
}

func (s *Service) fetchLatest(ctx context.Context, current buildinfo.Info) (*LatestInfo, bool, *Announcement, error) {
	var release struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		HTMLURL     string `json:"html_url"`
	}
	err := s.getJSON(ctx, "/repos/"+s.repository+"/releases/latest", &release)
	if err == nil {
		version := strings.TrimSpace(release.TagName)
		name := firstNonEmpty(strings.TrimSpace(release.Name), version)
		notes := truncateText(strings.TrimSpace(release.Body), 8000)
		latest := &LatestInfo{
			Version:     version,
			Name:        name,
			Notes:       notes,
			PublishedAt: strings.TrimSpace(release.PublishedAt),
			URL:         safeHTTPSURL(release.HTMLURL),
			Source:      "release",
		}
		announcement := normalizeAnnouncement(Announcement{
			ID:          "release-" + normalizeID(version),
			Type:        "update",
			Title:       "版本更新 " + firstNonEmpty(version, name),
			Summary:     firstNonEmpty(firstContentLine(notes), "GitHub 已发布新的项目版本。"),
			Content:     firstNonEmpty(notes, "GitHub 已发布新的项目版本。"),
			PublishedAt: latest.PublishedAt,
			URL:         latest.URL,
		})
		return latest, versionIsNewer(current.Version, version), &announcement, nil
	}
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusNotFound {
		return nil, false, nil, err
	}
	return s.fetchDefaultBranch(ctx, current)
}

func (s *Service) fetchDefaultBranch(ctx context.Context, current buildinfo.Info) (*LatestInfo, bool, *Announcement, error) {
	var repository struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := s.getJSON(ctx, "/repos/"+s.repository, &repository); err != nil {
		return nil, false, nil, fmt.Errorf("仓库没有 Release，读取默认分支失败：%w", err)
	}
	branch := strings.TrimSpace(repository.DefaultBranch)
	if branch == "" {
		branch = "main"
	}
	var commit struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message   string `json:"message"`
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := s.getJSON(ctx, "/repos/"+s.repository+"/commits/"+url.PathEscape(branch), &commit); err != nil {
		return nil, false, nil, fmt.Errorf("仓库没有 Release，读取最新提交失败：%w", err)
	}
	latestCommit := strings.TrimSpace(commit.SHA)
	currentCommit := strings.TrimSpace(current.Commit)
	knownCurrent := currentCommit != "" && currentCommit != "unknown"
	matches := knownCurrent && (strings.HasPrefix(latestCommit, currentCommit) || strings.HasPrefix(currentCommit, latestCommit))
	shortCommit := latestCommit
	if len(shortCommit) > 7 {
		shortCommit = shortCommit[:7]
	}
	message := firstContentLine(commit.Commit.Message)
	latest := &LatestInfo{
		Version:     current.Version,
		Name:        "GitHub 最新源码 " + shortCommit,
		Notes:       firstNonEmpty(message, "默认分支包含新的源码提交。"),
		PublishedAt: strings.TrimSpace(commit.Commit.Committer.Date),
		URL:         safeHTTPSURL(commit.HTMLURL),
		Source:      "commit",
		Commit:      latestCommit,
	}
	if !knownCurrent || matches {
		return latest, false, nil, nil
	}
	announcement := normalizeAnnouncement(Announcement{
		ID:          "commit-" + shortCommit,
		Type:        "update",
		Title:       "源码有新提交 " + shortCommit,
		Summary:     latest.Notes,
		Content:     "GitHub 默认分支已有新的源码提交，但目前还没有 Release 安装包。\n\n最新提交：" + latest.Notes,
		PublishedAt: latest.PublishedAt,
		URL:         latest.URL,
	})
	return latest, true, &announcement, nil
}

func (s *Service) fetchRepositoryAnnouncements(ctx context.Context) ([]Announcement, error) {
	var content struct {
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	err := s.getJSON(ctx, "/repos/"+s.repository+"/contents/internal/updatecheck/announcements.json", &content)
	if err != nil {
		var statusErr *httpStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(content.Encoding), "base64") {
		return nil, errors.New("公告文件编码不是 base64")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content.Content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("解析公告文件失败：%w", err)
	}
	var document announcementDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("公告文件格式错误：%w", err)
	}
	return normalizeAnnouncements(document.Announcements), nil
}

func (s *Service) getJSON(ctx context.Context, path string, target any) error {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "https://api.github.com"+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "iCloud-Privacy-Mail-v2-Updater/"+buildinfo.Current().Version)
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return &httpStatusError{StatusCode: response.StatusCode, Body: string(body)}
	}
	return json.NewDecoder(io.LimitReader(response.Body, responseMaxSize)).Decode(target)
}

func builtinAnnouncements() []Announcement {
	raw, err := announcementFiles.ReadFile("announcements.json")
	if err != nil {
		return []Announcement{}
	}
	var document announcementDocument
	if json.Unmarshal(raw, &document) != nil {
		return []Announcement{}
	}
	return normalizeAnnouncements(document.Announcements)
}

func mergeAnnouncements(groups ...[]Announcement) []Announcement {
	items := map[string]Announcement{}
	for _, group := range groups {
		for _, raw := range group {
			item := normalizeAnnouncement(raw)
			if item.ID == "" || item.Title == "" {
				continue
			}
			items[item.ID] = item
		}
	}
	result := make([]Announcement, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].PublishedAt > result[j].PublishedAt
	})
	return result
}

func normalizeAnnouncements(items []Announcement) []Announcement {
	return mergeAnnouncements(items)
}

func normalizeAnnouncement(item Announcement) Announcement {
	item.ID = truncateText(strings.TrimSpace(item.ID), 120)
	item.Type = strings.ToLower(strings.TrimSpace(item.Type))
	switch item.Type {
	case "update", "project", "system":
	default:
		item.Type = "project"
	}
	item.Title = truncateText(strings.TrimSpace(item.Title), 160)
	item.Summary = truncateText(strings.TrimSpace(item.Summary), 500)
	item.Content = truncateText(strings.TrimSpace(item.Content), 8000)
	item.PublishedAt = strings.TrimSpace(item.PublishedAt)
	item.URL = safeHTTPSURL(item.URL)
	return item
}

func validRepository(repository string) bool {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, part := range parts {
		for _, char := range part {
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
				continue
			}
			return false
		}
	}
	return true
}

func safeHTTPSURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}

func versionIsNewer(current, latest string) bool {
	current = normalizeVersion(current)
	latest = normalizeVersion(latest)
	if latest == "" {
		return false
	}
	if current == "" || strings.Contains(current, "dev") || strings.Contains(current, "unknown") {
		return current != latest
	}
	if current == latest {
		return false
	}
	currentParts := splitVersionParts(current)
	latestParts := splitVersionParts(latest)
	maxParts := len(currentParts)
	if len(latestParts) > maxParts {
		maxParts = len(latestParts)
	}
	for index := 0; index < maxParts; index++ {
		currentPart := 0
		latestPart := 0
		if index < len(currentParts) {
			currentPart = currentParts[index]
		}
		if index < len(latestParts) {
			latestPart = latestParts[index]
		}
		if latestPart != currentPart {
			return latestPart > currentPart
		}
	}
	return false
}

func normalizeVersion(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "version")
	value = strings.TrimSpace(value)
	return strings.TrimPrefix(value, "v")
}

func splitVersionParts(value string) []int {
	fields := strings.FieldsFunc(value, func(char rune) bool { return char < '0' || char > '9' })
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		part, err := strconv.Atoi(field)
		if err == nil {
			parts = append(parts, part)
		}
	}
	return parts
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstContentLine(value string) string {
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#-* "))
		if line != "" {
			return truncateText(line, 500)
		}
	}
	return ""
}

func normalizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(char rune) rune {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			return char
		}
		return '-'
	}, value)
	return strings.Trim(value, "-")
}

func truncateText(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "…"
}
