package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

type reactivePerson struct {
	ID   int
	Name string
}

func reactivityPreview() ggui.Widget {
	return preview("Reactive blocks", ggui.Component(func() ggui.Widget {
		query := ggui.State("")
		shown := ggui.State(true)
		cleaned := ggui.State(0)
		people := ggui.State([]reactivePerson{{1, "Ada"}, {2, "Grace"}, {3, "Linus"}})
		results := ggui.Resource(query.Get, func(ctx context.Context, q string) ([]reactivePerson, error) {
			timer := time.NewTimer(180 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
			}
			if strings.EqualFold(q, "error") {
				return nil, fmt.Errorf("Demo request failed; change the query or retry")
			}
			var matches []reactivePerson
			for _, person := range []reactivePerson{{1, "Ada"}, {2, "Grace"}, {3, "Linus"}} {
				if strings.Contains(strings.ToLower(person.Name), strings.ToLower(q)) {
					matches = append(matches, person)
				}
			}
			return matches, nil
		})
		row := func(item ggui.EachItem[reactivePerson]) ggui.Widget {
			selected := ggui.State(false)
			label := ggui.Derived(func() string {
				return fmt.Sprintf("%d. %s", item.Index.Get()+1, item.Value.Get().Name)
			})
			return ggui.Row(
				ui.Checkbox(selected, "Keep this row selected"),
				ggui.TextOf(label),
			).Gap(8)
		}
		return ggui.Column(
			ui.TextField(query).Name("Reactive search").Placeholder("Search names; type error to fail"),
			ui.Button("Retry request", results.Reload).Outline(),
			ggui.Await(results).
				Pending(func() ggui.Widget { return ggui.Text("Searching…") }).
				Then(func(value ggui.Readable[[]reactivePerson]) ggui.Widget {
					return ggui.EachKeyed(value, func(p reactivePerson) int { return p.ID }, func(item ggui.EachItem[reactivePerson]) ggui.Widget {
						return ggui.TextOf(ggui.Map(item.Value, func(p reactivePerson) string { return p.Name }))
					}).Else(func() ggui.Widget { return ggui.Text("No matches") })
				}).Catch(func(err ggui.Readable[error]) ggui.Widget { return ggui.Textf("%v", err) }),
			ui.Button("Reverse keyed rows", func() {
				people.Update(func(p []reactivePerson) []reactivePerson {
					out := slices.Clone(p)
					slices.Reverse(out)
					return out
				})
			}).Outline(),
			ggui.EachKeyed(people, func(p reactivePerson) int { return p.ID }, row).Gap(4),
			ui.Checkbox(shown, "Mount local counter"),
			ggui.If(shown, func() ggui.Widget {
				count := ggui.State(0)
				ggui.OnCleanup(func() { ggui.Add(cleaned, 1) })
				return ggui.Row(ggui.Textf("Local count: %d", count), ui.Button("Increment local", func() { ggui.Add(count, 1) })).Gap(8)
			}),
			ggui.Textf("Disposed branches: %d", cleaned),
		).Gap(8)
	}))
}
