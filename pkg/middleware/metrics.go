package middleware

import (
	"errors"
	"strconv"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ชุด metric ของ HTTP layer
//
// ชื่อ metric ตั้งให้ตรงกับ pet-service ทุกตัว เพื่อให้ dashboard เดียว
// ครอบได้ทุก service โดยแยกด้วย label `job` ที่ Prometheus ใส่ให้เอง
// ถ้าตั้งชื่อไม่ตรงกันจะต้องเขียน query แยกต่อ service ซึ่งพังทันทีที่มี service ใหม่
//
// ⚠️ ใช้ c.Route().Path เป็น label ไม่ใช่ c.Path() เพราะ path จริงมี id ปนอยู่
// ถ้าใช้ path ดิบ label จะแตกไม่จำกัดจน Prometheus ระเบิด (cardinality explosion)
var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "จำนวน HTTP request ทั้งหมด แยกตาม method / route / status",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "เวลาที่ใช้ต่อ request",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)

	httpRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "จำนวน request ที่กำลังทำงานอยู่",
		},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal, httpRequestDuration, httpRequestsInFlight)
}

// NewMetrics คืน middleware ที่เก็บ metric ของทุก request
func NewMetrics() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// ไม่นับตัวเอง — Prometheus ยิงทุก 30 วินาที ถ้านับด้วยกราฟ
		// จะมี traffic พื้นหลังตลอดเวลาทั้งที่ไม่มีใครใช้งานจริง
		if c.Path() == "/metrics" {
			return c.Next()
		}

		// ⚠️ ต้องคัดลอก string ที่ได้จาก c.Method() ก่อนเสมอ
		//
		// Fiber คืน string ที่ชี้ไป buffer ของ request ซึ่งถูกใช้ซ้ำกับ request
		// ถัดไป ส่วน Prometheus เก็บ string นั้นไว้เป็น key ตลอดอายุ process
		// พอ buffer ถูกเขียนทับ key ก็เปลี่ยนตาม
		//
		// เกิดขึ้นจริงกับ pet-service: label กลายเป็น "GETETE" (GET ทับด้วยเศษ
		// ของ DELETE) แล้ว label ซ้ำจน /metrics ตอบ 500 ทั้ง endpoint
		// Prometheus จึง scrape ไม่ผ่านเลยโดยที่ ServiceMonitor ยังเขียวอยู่
		method := utils.CopyString(c.Method())

		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			httpRequestDuration.WithLabelValues(method, routeLabel(c)).Observe(v)
		}))

		err := c.Next()

		// อ่าน status หลัง c.Next() เพราะ error handler อาจเปลี่ยน status
		status := c.Response().StatusCode()
		if err != nil {
			var fe *fiber.Error
			if errors.As(err, &fe) {
				status = fe.Code
			}
		}
		httpRequestsTotal.WithLabelValues(method, routeLabel(c), strconv.Itoa(status)).Inc()
		timer.ObserveDuration()

		return err
	}
}

// routeLabel คืน pattern ของ route ที่จับคู่ได้ ไม่ใช่ path จริง
// ถ้าไม่ตรง route ไหนเลย (404) ยุบเป็น "unmatched" เพื่อไม่ให้ label แตก
func routeLabel(c *fiber.Ctx) string {
	r := c.Route()
	if r == nil || r.Path == "" {
		return "unmatched"
	}
	// request ที่ไม่ตรง route ไหนเลย fiber จะคืน route ของ app.Use ซึ่ง Path = "/"
	// ถ้า pattern เป็น "/" แต่ path จริงไม่ใช่ "/" แปลว่าไม่ได้ match จริง
	if r.Path == "/" && c.Path() != "/" {
		return "unmatched"
	}
	return r.Path
}

// MetricsHandler คือ endpoint /metrics ให้ Prometheus มาดึง
func MetricsHandler() fiber.Handler {
	return adaptor.HTTPHandler(promhttp.Handler())
}
