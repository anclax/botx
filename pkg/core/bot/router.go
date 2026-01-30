package bot

import (
	"context"

	"github.com/cloudcarver/botx/pkg/core/session"
	"github.com/pkg/errors"
)

// Router is a wrapper of session store to manage routing history
type Router struct {
	chatID int64
	sess   session.Session
}

func CreateRouter(ctx context.Context, chatID int64, sess session.Session) (*Router, error) {
	if err := sess.Set(ctx, SessionKeyRouterHist, []string{"/"}); err != nil {
		return nil, errors.Wrap(err, "failed to create router history")
	}
	return &Router{chatID: chatID, sess: sess}, nil
}

func (r *Router) History(ctx context.Context) ([]string, error) {
	hist, err := r.sess.Get(ctx, SessionKeyRouterHist)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get router history")
	}
	return hist.([]string), nil
}

func (r *Router) Push(ctx context.Context, url string) error {
	hist, err := r.History(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get router history")
	}
	if url == hist[len(hist)-1] {
		return nil
	}

	hist = append(hist, url)
	if err := r.sess.Set(ctx, SessionKeyRouterHist, hist); err != nil {
		return errors.Wrap(err, "failed to set router history")
	}
	return nil
}

func (r *Router) Back(ctx context.Context) (string, error) {
	hist, err := r.History(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get router history")
	}
	if len(hist) > 1 {
		last := hist[len(hist)-1]
		hist = hist[:len(hist)-1]
		if err := r.sess.Set(ctx, SessionKeyRouterHist, hist); err != nil {
			return "", errors.Wrap(err, "failed to set router history")
		}
		return last, nil
	}
	return "/", nil
}
