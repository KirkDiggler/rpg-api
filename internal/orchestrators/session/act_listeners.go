package session

import (
	"context"
	"sort"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// The broker is the act stream the SDK commits through (sdk.Config.Acts), so
// it is also the stream's answer to who is listening.
var _ sdk.ActListeners = (*ActBroker)(nil)

// Listening implements sdk.ActListeners: the recipients of session with at
// least one live subscription right now, in name order. It is the write
// tier's audience answer (rpg-project#879 S7): the SDK's commit folds acts
// only for the members named here, so a member with no StreamEvents
// connection costs a commit nothing.
//
// NOBODY LOSES AN ACT BY NOT BEING NAMED. A member that is not listening
// reads its acts on catch-up (GetStory, and the replay StreamEvents makes),
// which folds from its own story on demand. That holds because StreamEvents
// SUBSCRIBES BEFORE IT READS CATCH-UP: a member counts as listening from the
// moment it subscribes, so a commit before that is in its catch-up read and
// a commit after it is folded live. A subscription that ends — closed by its
// stream, lagged, or by the broker's Close — stops counting at once.
//
// Read under the broker's mutex from the subscription map; it never errors.
func (b *ActBroker) Listening(_ context.Context, session string) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	listening := []string{}
	for key, subs := range b.subs {
		if key.session == session && len(subs) > 0 {
			listening = append(listening, key.recipient)
		}
	}
	sort.Strings(listening)
	return listening, nil
}
