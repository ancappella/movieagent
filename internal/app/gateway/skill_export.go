package gateway

const (
	TransferIdTimeoutSeconds = 120.0
	maxExportSlots = 2
	chunkSize = 1024 * 1024
)

var (
	ErrClientDisconnected = errors.New("export client disconnected")
	ErrExportBusy = &HTTPError {
		Status: http.StatusTooManyRequests,
		Code: "skill_export_busy",

	}
	slots = make(chan struct{}, maxExportSlots)
)

type SkillExportError struct {
	Status int
	Code string
	Message string
	Path string
}

func (e *SkillExportError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %s", e.Path, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type SkillExport struct {
	File io.ReadCloser
	Size int64
}

func (a *SkillExportArchive) Close() error {
	if a == nil || a.File == nil {
		return nil
	}
	return a.File.Close()
}

type HTTPError struct {
	Status int
	Code   string
	Message string
	Path    string
}

func (e *HTTPError) Error() string {
	return e.Message
}

func ExportHTTPError(err *SkillExportError) *HTTPError {
	if err == nil {
		return nil
	}
	return &HTTPError {
		Status: err.Status,
		Code: err.Code,
		Message: err.Message,
		Path: err.Path,
	}
}

func (e *HTTPError) Detail() map[string]string {
     detail := map[string]string {
		"code": e.Code,
		"message": e.Message,
	 }
	 if e.Path != "" {
		detail["path"] = e.Path
	 }
	 return detail
}

type SkillExportNotice struct {
	Code    string
	Message string
	Path    *string 
}

type SkillExportFile struct {
	Path       string     `json:"path"`
	Type       string     `json:"type"`
	Size       int64      `json:"size"`
	Executable bool       `json:"executable"`
}

type SkillExportRequirements struct {
	Compatibility *string `json:"compatibility"`
	AllowedTools  []string `json:"allowed_tools"`
	RequiredSecrets []SkillExportSecret `json:"required_secrets"`
}

type SkillExportManifestResponse struct {
	SkillName string `json:"skill_name"`
	Revision *string `json:"revision"`
	CanExport      bool `json:"can_export"`
	FileCount      int `json:"file_count"`
	DirectoryCount int `json:"directory_count"`
	TotalBytes   int64 `json:"total_bytes"`
	Files       []SkillExportFile `json:"files"`
	Directories []SkillExportDirectory `json:"directories"`
	Requirements *SkillExportRequirements `json:"requirements"`
}

type ExportLease struct {
	released atomic.Bool
}

func AcquireLease() (*ExportLease, error) {
	select {
	case slots <- struct{}{}:
		return &ExportLease{}, nil
	default:
		return nil, ErrExportBusy
	}
}

func (l *ExportLease) Release() {
	if l == nil {
		return 
	}
	if l.released.CompareAndSwap(false, true) {
		<-slots
	}
}

type CancelEvent struct {
	ch chan struct{}
	once sync.Once
    closed atomic.Bool
}

func NewCancelEvent() *CancelEvent {
	return &CancelEvent {
		ch: make(chan struct{}),
	}
}

func (e *CancelEvent) IsSet() bool {
	return e.closed.Load()
}

func (e *CancelEvent) Done() <- chan struct{} {
	return e.ch
}

type ExportWork func(cancel *CancelEvent) (any, error)

func RunExportWork(ctx context.Context, work ExportWork, disconnect <- chan struct{}) (any, *ExportLease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lease , err := AcquireLease()
	if err != nil {
		return nil , nil, err
	}
	cancel := NewCancelEvent() 
	type result struct {
		value any
		err error
	}
	done := make(chan result, 1)
	go func() {
		value , err := work(cancel)
		done <- result {value, err}
	}()
	fail := func(retErr error) (any, *ExportLease, error) {
		cancel.Set()
		r := <-done
		if r.err == nil {
			
		}
	}
	if disconnect != nil {
		select {
		case r := <- done:
			if r.err != nil {
				lease.Release()
				return nil, nil, r.err
			}
			return r.value, lease, nil
		case <- disconnect:
			return fail(ErrClientDisconnected)
		case <- ctx.Done():
			return fail(ctx.Err())
		}
	}
	select {
	case r := <- done:
		if r.err != nil {
			lease.Release()
			return nil, nil, r.err
		}
		return r.value, lease, nil
	case <- ctx.Done():
		return fail(ctx.Err())
	}
}