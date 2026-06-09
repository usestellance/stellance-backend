package pdf

import (
	"context"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// HTMLToPDF renders HTML bytes to a PDF binary using headless Chrome.
func HTMLToPDF(html []byte) ([]byte, error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-dev-shm-usage", true),
		)...,
	)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()

	dataURL := "data:text/html;charset=utf-8," + urlEncode(string(html))

	var pdfBuf []byte
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			pdfBuf, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithMarginTop(0.4).
				WithMarginBottom(0.4).
				WithMarginLeft(0.4).
				WithMarginRight(0.4).
				WithPaperWidth(8.5).
				WithPaperHeight(11).
				Do(ctx)
			return err
		}),
	); err != nil {
		return nil, err
	}

	return pdfBuf, nil
}

func urlEncode(s string) string {
	const hex = "0123456789ABCDEF"
	buf := make([]byte, 0, len(s)*3)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' ||
			c == '!' || c == '\'' || c == '(' || c == ')' || c == '*' {
			buf = append(buf, c)
		} else {
			buf = append(buf, '%', hex[c>>4], hex[c&0xf])
		}
	}
	return string(buf)
}
