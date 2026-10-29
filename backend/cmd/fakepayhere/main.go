// Command fakepayhere plays PayHere for a local API: it posts a signed notify
// for a payment, so the pay, replay, late and failure paths can be tried with no
// PayHere account (docs/09 section 2.3, docs/15). It signs with the merchant id
// and secret you give it in the environment, and it only talks to a loopback
// address, so it can never be pointed at a live API.
//
//	PAYHERE_MERCHANT_ID=1230001 PAYHERE_MERCHANT_SECRET=... \
//	  go run ./cmd/fakepayhere -payment <payment id> -amount 1585.00 [-status 2] [-times 2] [-card]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gp/internal/payment/payhere"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, http.DefaultClient); err != nil {
		fmt.Fprintln(os.Stderr, "fakepayhere:", err)
		os.Exit(1)
	}
}

type options struct {
	base, payment, amount, status, ref string
	times                              int
	card                               bool
}

func run(ctx context.Context, args []string, getenv func(string) string, client *http.Client) error {
	var o options
	fs := flag.NewFlagSet("fakepayhere", flag.ContinueOnError)
	fs.StringVar(&o.base, "base", "http://localhost:8080", "API base URL (loopback only)")
	fs.StringVar(&o.payment, "payment", "", "our payment id: the order_id of the checkout form")
	fs.StringVar(&o.amount, "amount", "", "amount as in the form, for example 1585.00")
	fs.StringVar(&o.status, "status", "2", "PayHere status_code: 2 success, 0 pending, -1 cancelled, -2 failed, -3 charged back")
	fs.StringVar(&o.ref, "ref", "320032315782", "PayHere's own payment id")
	fs.IntVar(&o.times, "times", 1, "send the same notify this many times (replay)")
	fs.BoolVar(&o.card, "card", false, "add card fields, which the API must not keep")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if o.payment == "" || o.amount == "" || o.times < 1 {
		return errors.New("usage: fakepayhere -payment ID -amount 1585.00 [-status 2] [-times N] [-card]")
	}
	merchant, secret := getenv("PAYHERE_MERCHANT_ID"), getenv("PAYHERE_MERCHANT_SECRET")
	if merchant == "" || secret == "" {
		return errors.New("set PAYHERE_MERCHANT_ID and PAYHERE_MERCHANT_SECRET")
	}
	endpoint, err := notifyURL(o.base)
	if err != nil {
		return err
	}
	form := notifyForm(o, merchant, secret)
	for i := 1; i <= o.times; i++ {
		code, err := post(ctx, client, endpoint, form)
		if err != nil {
			return err
		}
		fmt.Printf("notify %d of %d: HTTP %d\n", i, o.times, code)
	}
	return nil
}

// notifyURL refuses anything but a loopback host: this tool signs with a real
// merchant secret and must never reach a live API.
func notifyURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "", errors.New("-base is not a URL")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", errors.New("-base must be localhost or a loopback address")
	}
	return strings.TrimRight(base, "/") + "/v1/webhooks/payhere", nil
}

// notifyForm is what PayHere posts, signed the way the API checks it.
func notifyForm(o options, merchant, secret string) url.Values {
	v := url.Values{
		"merchant_id": {merchant}, "order_id": {o.payment}, "payment_id": {o.ref}, "payhere_amount": {o.amount},
		"payhere_currency": {"LKR"}, "status_code": {o.status}, "method": {"VISA"}, "status_message": {"fake payhere"},
	}
	v.Set("md5sig", payhere.NotifySignature(merchant, o.payment, o.amount, "LKR", o.status, secret))
	if o.card {
		v.Set("card_holder_name", "Test Person")
		v.Set("card_no", "************1234")
		v.Set("card_expiry", "12/30")
	}
	return v
}

func post(ctx context.Context, client *http.Client, endpoint string, form url.Values) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("post notify: %w", err)
	}
	defer res.Body.Close()
	return res.StatusCode, nil
}
