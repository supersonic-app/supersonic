package dialogs

import (
	"context"
	"fmt"
	"log"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"github.com/deluan/sanitize"
	"github.com/supersonic-app/supersonic/backend"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
	"github.com/supersonic-app/supersonic/sharedutil"
	"github.com/supersonic-app/supersonic/ui/theme"
	"github.com/supersonic-app/supersonic/ui/util"
)

// AutoEQBrowser allows users to browse and select AutoEQ headphone profiles
type AutoEQBrowser struct {
	SearchDialog      *SearchDialog
	manager           *backend.AutoEQManager
	toastProvider     ToastProvider
	allProfileResults []*mediaprovider.SearchResult
	OnProfileSelected func(*backend.AutoEQProfile)
}

func NewAutoEQBrowser(manager *backend.AutoEQManager, im util.ImageFetcher, toastProvider ToastProvider) *AutoEQBrowser {
	ab := &AutoEQBrowser{
		manager:       manager,
		toastProvider: toastProvider,
	}
	sd := NewSearchDialog(
		im,
		lang.L("Browse Headphone Profiles"),
		lang.L("Cancel"),
		ab.onSearched,
	)
	sd.PlaceholderText = lang.L("Search headphones...")
	ab.SearchDialog = sd
	return ab
}

func (ab *AutoEQBrowser) fetchAllProfiles() error {
	ctx := context.Background()
	profiles, err := ab.manager.FetchIndex(ctx)
	if err != nil {
		// Show empty results on error
		ab.allProfileResults = []*mediaprovider.SearchResult{}
		return fmt.Errorf("failed to fetch AutoEQ index: %w", err)
	}

	// Convert to SearchResult format for display
	ab.allProfileResults = sharedutil.MapSlice(profiles, ab.profileToSearchResult)
	return nil
}

func (ab *AutoEQBrowser) profileToSearchResult(profile backend.AutoEQProfileMetadata) *mediaprovider.SearchResult {
	// Format secondary text as "type · source" (e.g., "over-ear · oratory1990")
	subtitle := ""
	if profile.Type != "" {
		subtitle = profile.Type
	}
	if profile.Source != "" {
		if subtitle != "" {
			subtitle += " · "
		}
		subtitle += profile.Source
	}

	return &mediaprovider.SearchResult{
		Name:       profile.Name,
		Icon:       theme.HeadphonesIcon,           // Use headphone icon for all profiles
		ID:         profile.Path,                   // Store path as ID for retrieval
		Type:       mediaprovider.ContentTypeOther, // Use "Other" content type for AutoEQ profiles
		ArtistName: subtitle,
		Size:       0, // Don't show track count
	}
}

func (ab *AutoEQBrowser) onSearched(query string) []*mediaprovider.SearchResult {
	if ab.allProfileResults == nil {
		if err := ab.fetchAllProfiles(); err != nil {
			log.Printf("Failed to load AutoEQ profiles: %v", err)
			fyne.Do(func() {
				ab.toastProvider.ShowErrorToast(lang.L("Error loading AutoEQ profiles"))
			})
			return []*mediaprovider.SearchResult{}
		}
	}

	if query == "" {
		return ab.allProfileResults
	}

	// Filter by name (case-insensitive, accent-insensitive)
	return sharedutil.FilterSlice(ab.allProfileResults, func(result *mediaprovider.SearchResult) bool {
		return strings.Contains(
			sanitize.Accents(strings.ToLower(result.Name)),
			sanitize.Accents(strings.ToLower(query)),
		)
	})
}

func (ab *AutoEQBrowser) SetOnDismiss(onDismiss func()) {
	ab.SearchDialog.OnDismiss = onDismiss
}

func (ab *AutoEQBrowser) SetOnProfileSelected(callback func(*backend.AutoEQProfile)) {
	ab.OnProfileSelected = callback
	ab.SearchDialog.OnNavigateTo = func(_ mediaprovider.ContentType, profilePath string) {
		go func() {
			// Fetch the full profile data
			profile, err := ab.manager.FetchProfile(context.Background(), profilePath)
			fyne.Do(func() {
				if err != nil {
					log.Printf("Error loading AutoEQ profile: %v", err)
					ab.toastProvider.ShowErrorToast(lang.L("Error loading AutoEQ profile"))
				} else {
					if ab.OnProfileSelected != nil {
						ab.OnProfileSelected(profile)
					}
				}
			})
		}()
	}
}

func (ab *AutoEQBrowser) MinSize() fyne.Size {
	return ab.SearchDialog.MinSize()
}

func (ab *AutoEQBrowser) GetSearchEntry() fyne.Focusable {
	return ab.SearchDialog.GetSearchEntry()
}

func (ab *AutoEQBrowser) Show() {
	ab.SearchDialog.Show()
}

func (ab *AutoEQBrowser) Hide() {
	ab.SearchDialog.Hide()
}

func (ab *AutoEQBrowser) Refresh() {
	ab.SearchDialog.Refresh()
}
