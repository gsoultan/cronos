package vault

import (
	"context"
	"fmt"
)

/*
Reseal seals again, under the current key, every stored value a retired key
sealed — the second half of rotating CRONOS_SECRETS_KEY.

Run at startup. Lost is every value no configured key opens, by
org/project/name: a key replaced without keeping the old one as a previous key,
which is otherwise discovered one broken map at a time.

Each replacement is conditional on the value being the one that was read, so an
instance resealing at the same moment somebody sets a new password does not put
the old one back.
*/
func Reseal(ctx context.Context, store Resealing, sealer Sealer) (resealed int, lost []string, err error) {
	all, err := store.AllSecrets(ctx)
	if err != nil {
		return 0, nil, err
	}
	for _, st := range all {
		if !sealer.Stale(st.Sealed) {
			continue
		}
		v, err := sealer.Open(st.Sealed, st.Org, st.Project, st.Name)
		if err != nil {
			lost = append(lost, st.Org+"/"+st.Project+"/"+st.Name)
			continue
		}
		was := st.Sealed
		st.Sealed = sealer.Seal(v, st.Org, st.Project, st.Name)
		ok, err := store.Reseal(ctx, st, was)
		if err != nil {
			return resealed, lost, fmt.Errorf("resealing %s/%s/%s: %w", st.Org, st.Project, st.Name, err)
		}
		if ok {
			resealed++
		}
	}
	return resealed, lost, nil
}
