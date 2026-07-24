package upload

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/claudiovaldi/order-mock-payment/internal/order"
)

// --- Test doubles ---

// fakeOrderService satisfies upload.OrderService.
type fakeOrderService struct {
	orders map[uuid.UUID]*order.Order
	err    error
}

func newFakeOrderService() *fakeOrderService {
	return &fakeOrderService{orders: map[uuid.UUID]*order.Order{}}
}

func (s *fakeOrderService) Get(_ context.Context, userID, orderID uuid.UUID) (*order.Order, error) {
	if s.err != nil {
		return nil, s.err
	}
	o, ok := s.orders[orderID]
	if !ok || o.UserID != userID {
		return nil, order.ErrOrderNotFound
	}
	cp := *o
	return &cp, nil
}

func (s *fakeOrderService) seedOrder(userID uuid.UUID) *order.Order {
	o := &order.Order{
		ID:        uuid.New(),
		UserID:    userID,
		Symbol:    "BTCUSD",
		Side:      order.SideBuy,
		Status:    order.StatusPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	s.orders[o.ID] = o
	return o
}

// fakeRepo — in-memory Repository.
type fakeRepo struct {
	uploads   map[uuid.UUID]*Upload
	createErr error
	getErr    error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{uploads: map[uuid.UUID]*Upload{}}
}

func (r *fakeRepo) Create(_ context.Context, u *Upload) error {
	if r.createErr != nil {
		return r.createErr
	}
	u.ID = uuid.New()
	u.CreatedAt = time.Now().UTC()
	cp := *u
	r.uploads[u.ID] = &cp
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, _, uploadID uuid.UUID) (*Upload, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	u, ok := r.uploads[uploadID]
	if !ok {
		return nil, ErrUploadNotFound
	}
	cp := *u
	return &cp, nil
}

// fakeStorage — in-memory Storage.
type fakeStorage struct {
	saved   map[string][]byte
	saveErr error
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{saved: map[string][]byte{}}
}

func (s *fakeStorage) Save(_ context.Context, file multipart.File, filename string) (string, error) {
	if s.saveErr != nil {
		return "", s.saveErr
	}
	buf := &bytes.Buffer{}
	if _, err := buf.ReadFrom(file); err != nil {
		return "", err
	}
	s.saved[filename] = buf.Bytes()
	return "test/" + filename, nil
}

var (
	_ Repository   = (*fakeRepo)(nil)
	_ Storage      = (*fakeStorage)(nil)
	_ OrderService = (*fakeOrderService)(nil)
)

// --- File magic-byte helpers ---
//
// http.DetectContentType relies on well-known sniff signatures.
// Minimum viable payloads for each supported type:

var (
	pdfMagic  = []byte("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	pngMagic  = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	jpegMagic = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	textBytes = []byte("this is plain text, not a supported upload type\n")
)

// wrapReader adapts a *bytes.Reader into multipart.File.
type readSeekerFile struct{ *bytes.Reader }

func (readSeekerFile) Close() error { return nil }

func toMultipart(b []byte) multipart.File {
	return readSeekerFile{bytes.NewReader(b)}
}

// --- Tests ---

func TestCreate_Cases(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()
	storageBoom := errors.New("disk full")
	repoBoom := errors.New("db down")

	cases := []struct {
		name       string
		setup      func(orders *fakeOrderService, repo *fakeRepo, storage *fakeStorage) (uuid.UUID, uuid.UUID, []byte)
		size       int64 // if 0, use len(bytes)
		maxSize    int64
		wantErrIs  error
		wantStored bool
	}{
		{
			name: "success pdf",
			setup: func(o *fakeOrderService, _ *fakeRepo, _ *fakeStorage) (uuid.UUID, uuid.UUID, []byte) {
				ord := o.seedOrder(userA)
				return userA, ord.ID, pdfMagic
			},
			maxSize:    1024,
			wantStored: true,
		},
		{
			name: "success png",
			setup: func(o *fakeOrderService, _ *fakeRepo, _ *fakeStorage) (uuid.UUID, uuid.UUID, []byte) {
				ord := o.seedOrder(userA)
				return userA, ord.ID, pngMagic
			},
			maxSize:    1024,
			wantStored: true,
		},
		{
			name: "success jpeg",
			setup: func(o *fakeOrderService, _ *fakeRepo, _ *fakeStorage) (uuid.UUID, uuid.UUID, []byte) {
				ord := o.seedOrder(userA)
				return userA, ord.ID, jpegMagic
			},
			maxSize:    1024,
			wantStored: true,
		},
		{
			name: "unsupported content type (plain text)",
			setup: func(o *fakeOrderService, _ *fakeRepo, _ *fakeStorage) (uuid.UUID, uuid.UUID, []byte) {
				ord := o.seedOrder(userA)
				return userA, ord.ID, textBytes
			},
			maxSize:   1024,
			wantErrIs: ErrUnsupportedContentType,
		},
		{
			name: "oversized",
			setup: func(o *fakeOrderService, _ *fakeRepo, _ *fakeStorage) (uuid.UUID, uuid.UUID, []byte) {
				ord := o.seedOrder(userA)
				return userA, ord.ID, pdfMagic
			},
			size:      2048,
			maxSize:   1024,
			wantErrIs: ErrFileTooLarge,
		},
		{
			name: "empty file",
			setup: func(o *fakeOrderService, _ *fakeRepo, _ *fakeStorage) (uuid.UUID, uuid.UUID, []byte) {
				ord := o.seedOrder(userA)
				return userA, ord.ID, []byte{}
			},
			size:      0,
			maxSize:   1024,
			wantErrIs: ErrEmptyFile,
		},
		{
			name: "order not found",
			setup: func(_ *fakeOrderService, _ *fakeRepo, _ *fakeStorage) (uuid.UUID, uuid.UUID, []byte) {
				return userA, uuid.New(), pdfMagic
			},
			maxSize:   1024,
			wantErrIs: ErrOrderNotFound,
		},
		{
			name: "foreign order",
			setup: func(o *fakeOrderService, _ *fakeRepo, _ *fakeStorage) (uuid.UUID, uuid.UUID, []byte) {
				ord := o.seedOrder(userB) // owned by B
				return userA, ord.ID, pdfMagic
			},
			maxSize:   1024,
			wantErrIs: ErrOrderNotFound,
		},
		{
			name: "storage failure",
			setup: func(o *fakeOrderService, _ *fakeRepo, s *fakeStorage) (uuid.UUID, uuid.UUID, []byte) {
				ord := o.seedOrder(userA)
				s.saveErr = storageBoom
				return userA, ord.ID, pdfMagic
			},
			maxSize:   1024,
			wantErrIs: storageBoom,
		},
		{
			name: "repository failure",
			setup: func(o *fakeOrderService, r *fakeRepo, _ *fakeStorage) (uuid.UUID, uuid.UUID, []byte) {
				ord := o.seedOrder(userA)
				r.createErr = repoBoom
				return userA, ord.ID, pdfMagic
			},
			maxSize:   1024,
			wantErrIs: repoBoom,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orders := newFakeOrderService()
			repo := newFakeRepo()
			storage := newFakeStorage()
			userID, orderID, payload := tc.setup(orders, repo, storage)

			svc := NewService(repo, storage, orders, tc.maxSize)

			size := tc.size
			if size == 0 && tc.wantErrIs != ErrEmptyFile {
				size = int64(len(payload))
			}

			got, err := svc.Create(context.Background(), userID, orderID, toMultipart(payload), size)

			if tc.wantErrIs != nil {
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("err = %v, want errors.Is(_, %v)", err, tc.wantErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got.ID == uuid.Nil {
				t.Error("ID not populated")
			}
			if got.OrderID != orderID {
				t.Errorf("OrderID = %v, want %v", got.OrderID, orderID)
			}
			if !strings.HasSuffix(got.Filename, ".pdf") &&
				!strings.HasSuffix(got.Filename, ".png") &&
				!strings.HasSuffix(got.Filename, ".jpg") {
				t.Errorf("unexpected filename extension: %s", got.Filename)
			}
			if got.Path == "" {
				t.Error("Path not populated")
			}
			if !tc.wantStored {
				return
			}
			if len(storage.saved) != 1 {
				t.Errorf("storage saved %d files, want 1", len(storage.saved))
			}
		})
	}
}

func TestGet_DelegatesToRepository(t *testing.T) {
	userA := uuid.New()

	orders := newFakeOrderService()
	repo := newFakeRepo()
	storage := newFakeStorage()
	svc := NewService(repo, storage, orders, 1024)

	// Seed a stored upload directly via repo (bypassing full Create for brevity).
	u := &Upload{OrderID: uuid.New(), Filename: "x.pdf", ContentType: "application/pdf", Size: 100, Path: "test/x.pdf"}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Get(context.Background(), userA, u.ID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("got ID = %v, want %v", got.ID, u.ID)
	}

	_, err = svc.Get(context.Background(), userA, uuid.New())
	if !errors.Is(err, ErrUploadNotFound) {
		t.Errorf("unknown id err = %v, want ErrUploadNotFound", err)
	}
}

// --- Storage safety (defense-in-depth) ---

func TestLocalStorage_RejectsUnsafeFilenames(t *testing.T) {
	tmp := t.TempDir()
	s := NewLocalStorage(tmp)
	for _, bad := range []string{"", "../evil", "sub/x.pdf", "..\\evil", "..", "a..b/c"} {
		_, err := s.Save(context.Background(), toMultipart(pdfMagic), bad)
		if !errors.Is(err, ErrInvalidStoragePath) {
			t.Errorf("Save(%q) err = %v, want ErrInvalidStoragePath", bad, err)
		}
	}
}

func TestLocalStorage_SaveWritesFile(t *testing.T) {
	tmp := t.TempDir()
	s := NewLocalStorage(tmp)

	rel, err := s.Save(context.Background(), toMultipart(pdfMagic), "abc.pdf")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if rel == "" {
		t.Fatal("empty relative path")
	}
	// The file must exist under tmp/rel with the same bytes.
	got, err := os.ReadFile(tmp + string(os.PathSeparator) + rel)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if !bytes.Equal(got, pdfMagic) {
		t.Errorf("written bytes differ from source")
	}
}
