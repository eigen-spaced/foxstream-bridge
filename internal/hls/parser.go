package hls

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/grafov/m3u8"
)

// Segment represents a single HLS segment, optionally with byte-range info.
type Segment struct {
	URL        string
	ByteOffset int64 // -1 if not a byte-range segment
	ByteLength int64
}

// Quality mirrors the extension's quality variant for matching.
type Quality struct {
	Label     string
	Bandwidth int
	URL       string
}

// FetchAndParseMediaPlaylist fetches an m3u8 URL and extracts segments.
func FetchAndParseMediaPlaylist(playlistURL string, headers map[string]string, cookies string) ([]Segment, error) {
	body, err := FetchPlaylist(playlistURL, headers, cookies)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	playlist, listType, err := m3u8.DecodeFrom(body, true)
	if err != nil {
		return nil, fmt.Errorf("failed to parse m3u8: %w", err)
	}

	switch listType {
	case m3u8.MEDIA:
		mediaPlaylist := playlist.(*m3u8.MediaPlaylist)
		return ExtractSegments(mediaPlaylist, playlistURL)

	case m3u8.MASTER:
		masterPlaylist := playlist.(*m3u8.MasterPlaylist)
		if len(masterPlaylist.Variants) == 0 {
			return nil, fmt.Errorf("master playlist has no variants")
		}
		variantURL, err := ResolveURL(playlistURL, masterPlaylist.Variants[0].URI)
		if err != nil {
			return nil, err
		}
		return FetchAndParseMediaPlaylist(variantURL, headers, cookies)

	default:
		return nil, fmt.Errorf("unknown playlist type")
	}
}

// ExtractSegments pulls segments from a media playlist, including byte-range and init map info.
func ExtractSegments(pl *m3u8.MediaPlaylist, baseURL string) ([]Segment, error) {
	var segments []Segment

	if pl.Map != nil && pl.Map.URI != "" {
		mapURL, err := ResolveURL(baseURL, pl.Map.URI)
		if err != nil {
			return nil, fmt.Errorf("invalid map URL %q: %w", pl.Map.URI, err)
		}
		seg := Segment{URL: mapURL, ByteOffset: -1}
		if pl.Map.Limit > 0 {
			seg.ByteOffset = pl.Map.Offset
			seg.ByteLength = pl.Map.Limit
		}
		segments = append(segments, seg)
	}

	for _, s := range pl.Segments {
		if s == nil {
			continue
		}
		absURL, err := ResolveURL(baseURL, s.URI)
		if err != nil {
			return nil, fmt.Errorf("invalid segment URL %q: %w", s.URI, err)
		}
		seg := Segment{URL: absURL, ByteOffset: -1}
		if s.Limit > 0 {
			seg.ByteOffset = s.Offset
			seg.ByteLength = s.Limit
		}
		segments = append(segments, seg)
	}

	if len(segments) == 0 {
		return nil, fmt.Errorf("playlist contains no segments")
	}
	return segments, nil
}

// FindSelectedVariantURL returns the sub-playlist URL for the user's selected quality.
func FindSelectedVariantURL(masterURL, selectedQuality string, qualities []Quality, headers map[string]string, cookies string) (string, error) {
	for _, q := range qualities {
		if q.Label == selectedQuality && q.URL != "" {
			return ResolveURL(masterURL, q.URL)
		}
	}

	body, err := FetchPlaylist(masterURL, headers, cookies)
	if err != nil {
		return "", err
	}
	defer body.Close()

	playlist, listType, err := m3u8.DecodeFrom(body, true)
	if err != nil {
		return "", fmt.Errorf("failed to parse master playlist: %w", err)
	}

	if listType != m3u8.MASTER {
		return masterURL, nil
	}

	master := playlist.(*m3u8.MasterPlaylist)
	if len(master.Variants) == 0 {
		return "", fmt.Errorf("master playlist has no variants")
	}

	best := master.Variants[0]
	for _, v := range master.Variants[1:] {
		if v.Bandwidth > best.Bandwidth {
			best = v
		}
	}

	return ResolveURL(masterURL, best.URI)
}

// ParseDemuxedPair parses a master playlist and returns (videoPlaylistURL, audioPlaylistURL).
func ParseDemuxedPair(masterURL, selectedQuality string, qualities []Quality, headers map[string]string, cookies string) (string, string, error) {
	body, err := FetchPlaylist(masterURL, headers, cookies)
	if err != nil {
		return "", "", fmt.Errorf("fetch master: %w", err)
	}
	defer body.Close()

	playlist, listType, err := m3u8.DecodeFrom(body, true)
	if err != nil {
		return "", "", fmt.Errorf("parse master: %w", err)
	}

	if listType != m3u8.MASTER {
		return "", "", fmt.Errorf("expected master playlist, got media playlist")
	}

	master := playlist.(*m3u8.MasterPlaylist)
	if len(master.Variants) == 0 {
		return "", "", fmt.Errorf("master playlist has no variants")
	}

	var bestVariant *m3u8.Variant

	for _, q := range qualities {
		if q.Label == selectedQuality && q.URL != "" {
			resolved, err := ResolveURL(masterURL, q.URL)
			if err == nil {
				for _, v := range master.Variants {
					vURL, _ := ResolveURL(masterURL, v.URI)
					if vURL == resolved {
						bestVariant = v
						break
					}
				}
			}
			break
		}
	}

	if bestVariant == nil {
		for _, v := range master.Variants {
			if v.Resolution == "" {
				continue
			}
			if bestVariant == nil || v.Bandwidth > bestVariant.Bandwidth {
				bestVariant = v
			}
		}
		if bestVariant == nil {
			bestVariant = master.Variants[0]
			for _, v := range master.Variants[1:] {
				if v.Bandwidth > bestVariant.Bandwidth {
					bestVariant = v
				}
			}
		}
	}

	videoURL, err := ResolveURL(masterURL, bestVariant.URI)
	if err != nil {
		return "", "", fmt.Errorf("resolve video URL: %w", err)
	}

	audioURL := ""
	audioGroup := bestVariant.Audio

	if bestVariant.Alternatives != nil {
		for _, a := range bestVariant.Alternatives {
			if a.Type == "AUDIO" && a.URI != "" {
				if audioGroup == "" || a.GroupId == audioGroup {
					audioURL, err = ResolveURL(masterURL, a.URI)
					if err != nil {
						continue
					}
					break
				}
			}
		}
	}

	if audioURL == "" {
		for _, v := range master.Variants {
			for _, a := range v.Alternatives {
				if a.Type == "AUDIO" && a.URI != "" {
					if audioGroup == "" || a.GroupId == audioGroup {
						audioURL, err = ResolveURL(masterURL, a.URI)
						if err != nil {
							continue
						}
						break
					}
				}
			}
			if audioURL != "" {
				break
			}
		}
	}

	if audioURL == "" {
		return "", "", fmt.Errorf("no audio rendition found in master playlist")
	}

	return videoURL, audioURL, nil
}

// FetchPlaylist fetches an m3u8 playlist with optional headers and cookies.
func FetchPlaylist(playlistURL string, headers map[string]string, cookies string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", playlistURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d fetching playlist", resp.StatusCode)
	}
	return resp.Body, nil
}

// ResolveURL resolves a relative URL against a base URL.
func ResolveURL(base, ref string) (string, error) {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref, nil
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return baseURL.ResolveReference(refURL).String(), nil
}
