package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("usersettings serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &user.Settings{
			ID:                             1,
			UserID:                         2,
			AutoCloseBrackets:              true,
			HighlightLine:                  true,
			Keybindings:                    "vim",
			MatchBrackets:                  false,
			RulerColumn:                    80,
			ShowLineNumbers:                true,
			ShowTrailingSpace:              true,
			SoftTabs:                       true,
			TabSize:                        4,
			Theme:                          "base16-dark",
			WrapLines:                      true,
			RemoveTrailingWhitespaceOnSave: true,
			CreatedAt:                      pq.NullTime{createdAt, true}, // Not serialized.
			UpdatedAt:                      pq.NullTime{updatedAt, true}, // Not serialized.
		}
	)

	Describe("SerializeUserSettings", func() {
		It("serializes to JSON", func() {
			p := serializers.SerializeUserSettings(model)
			b, err := json.Marshal(p)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 1,
				"user_id": 2,
				"auto_close_brackets": true,
				"highlight_line": true,
				"keybindings": "vim",
				"match_brackets": false,
				"ruler_column": 80,
				"show_line_numbers": true,
				"show_trailing_space": true,
				"soft_tabs": true,
				"tab_size": 4,
				"theme": "base16-dark",
				"wrap_lines": true,
				"remove_trailing_whitespace_on_save": true
			}`))
		})
	})
})
