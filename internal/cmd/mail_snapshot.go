package cmd

import "github.com/steveyegge/gastown/internal/mail"

type mailSnapshotLister interface {
	List() ([]*mail.Message, error)
}

type mailSnapshot struct {
	messages    []*mail.Message
	unread      []*mail.Message
	total       int
	unreadCount int
}

func loadMailSnapshot(lister mailSnapshotLister) (mailSnapshot, error) {
	messages, err := lister.List()
	if err != nil {
		return mailSnapshot{}, err
	}
	if messages == nil {
		messages = make([]*mail.Message, 0)
	}

	unread := make([]*mail.Message, 0)
	for _, msg := range messages {
		if msg != nil && !msg.Read {
			unread = append(unread, msg)
		}
	}

	return mailSnapshot{
		messages:    messages,
		unread:      unread,
		total:       len(messages),
		unreadCount: len(unread),
	}, nil
}
