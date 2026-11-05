package rows_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain/rows"
)

func TestGroupEpisodes(t *testing.T) {
	t.Parallel()

	day := func(number int) time.Time { return time.Date(2026, time.November, number, 0, 0, 0, 0, time.UTC) }

	tests := []struct {
		name   string
		groups []rows.EpisodeGroup
		limit  int
		want   []rows.EpisodeCard
	}{
		{name: "no groups", groups: nil, limit: 5, want: []rows.EpisodeCard{}},
		{
			name:   "one season becomes a season card",
			groups: []rows.EpisodeGroup{{ShowID: "show-a", SeasonID: "a-s1", EpisodeCount: 3, NewestAdded: day(2)}},
			limit:  5,
			want:   []rows.EpisodeCard{{ItemID: "a-s1", ShowID: "show-a", Season: true, NewEpisodeCount: 3, NewestAdded: day(2)}},
		},
		{
			name: "two seasons become a show card",
			groups: []rows.EpisodeGroup{
				{ShowID: "show-a", SeasonID: "a-s1", EpisodeCount: 2, NewestAdded: day(1)},
				{ShowID: "show-a", SeasonID: "a-s2", EpisodeCount: 4, NewestAdded: day(3)},
			},
			limit: 5,
			want:  []rows.EpisodeCard{{ItemID: "show-a", ShowID: "show-a", NewEpisodeCount: 6, NewestAdded: day(3)}},
		},
		{
			name: "cards ordered by newest episode, ties by show ID",
			groups: []rows.EpisodeGroup{
				{ShowID: "show-b", SeasonID: "b-s1", EpisodeCount: 1, NewestAdded: day(1)},
				{ShowID: "show-c", SeasonID: "c-s1", EpisodeCount: 1, NewestAdded: day(4)},
				{ShowID: "show-a", SeasonID: "a-s1", EpisodeCount: 1, NewestAdded: day(4)},
			},
			limit: 5,
			want: []rows.EpisodeCard{
				{ItemID: "a-s1", ShowID: "show-a", Season: true, NewEpisodeCount: 1, NewestAdded: day(4)},
				{ItemID: "c-s1", ShowID: "show-c", Season: true, NewEpisodeCount: 1, NewestAdded: day(4)},
				{ItemID: "b-s1", ShowID: "show-b", Season: true, NewEpisodeCount: 1, NewestAdded: day(1)},
			},
		},
		{
			name: "limit keeps the newest",
			groups: []rows.EpisodeGroup{
				{ShowID: "show-a", SeasonID: "a-s1", EpisodeCount: 1, NewestAdded: day(1)},
				{ShowID: "show-b", SeasonID: "b-s1", EpisodeCount: 1, NewestAdded: day(2)},
			},
			limit: 1,
			want:  []rows.EpisodeCard{{ItemID: "b-s1", ShowID: "show-b", Season: true, NewEpisodeCount: 1, NewestAdded: day(2)}},
		},
		{
			name:   "empty groups are ignored",
			groups: []rows.EpisodeGroup{{ShowID: "show-a", SeasonID: "a-s1", EpisodeCount: 0, NewestAdded: day(1)}},
			limit:  5,
			want:   []rows.EpisodeCard{},
		},
		{
			name:   "zero limit",
			groups: []rows.EpisodeGroup{{ShowID: "show-a", SeasonID: "a-s1", EpisodeCount: 1, NewestAdded: day(1)}},
			limit:  0,
			want:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := rows.GroupEpisodes(test.groups, test.limit); !reflect.DeepEqual(got, test.want) {
				t.Errorf("GroupEpisodes = %+v, want %+v", got, test.want)
			}
		})
	}
}
