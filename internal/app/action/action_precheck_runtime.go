package action

import "context"

func runStandardActionPrecheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	if uc.SessionRepo != nil {
		if err := uc.SessionRepo.EnsureActive(ctx, ac.In.SessionID, ac.In.AgentID, ac.View.StateWorking.Version); err != nil {
			return err
		}
	}
	intent := ac.Tmp.ResolvedIntent
	if err := ensureCooldownReady(ac.View.EventsBefore, intent.Type, ac.In.NowAt); err != nil {
		return err
	}
	return nil
}
