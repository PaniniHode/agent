package main

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"syscall"

	"github.com/golang/snappy"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/prometheus/prompb"
)

// main handles creating the benchmark.
func main() {
	flags()
}
func buildAgent() {
	cmd := exec.Command("go", "build", "../grafana-agent-flow")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		panic(err.Error())
	}
}

func cleanupPid(pid *exec.Cmd, dir string) {
	_ = pid.Process.Kill()
	_ = pid.Process.Release()
	_ = pid.Wait()
	_ = syscall.Kill(-pid.Process.Pid, syscall.SIGKILL)
	_ = os.RemoveAll(dir)
}

var networkdown = true

var totalSeries = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "benchmark_series_received",
}, []string{"from"})

var totalErrors = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "benchmark_errors",
}, []string{"from"})

func httpServer() {
	r := mux.NewRouter()
	r.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		handlePost(w, r)
	})
	r.HandleFunc("/allow", func(w http.ResponseWriter, r *http.Request) {
		println("allowing")
		networkdown = true
	})
	r.HandleFunc("/block", func(w http.ResponseWriter, r *http.Request) {
		println("blocking")
		networkdown = false
	})
	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/", r)
	println("Starting server")
	err := http.ListenAndServe(":8888", nil)
	if err != nil {
		println(err)
	}
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	if networkdown {
		println("returning 500")
		w.WriteHeader(500)
		return
	} else {
		defer r.Body.Close()
		from := r.Header.Get("from")
		data, err := io.ReadAll(r.Body)
		if err != nil {
			totalErrors.WithLabelValues(from).Inc()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data, err = snappy.Decode(nil, data)
		if err != nil {
			totalErrors.WithLabelValues(from).Inc()
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req prompb.WriteRequest
		if err := req.Unmarshal(data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, x := range req.GetTimeseries() {
			totalSeries.WithLabelValues(from).Add(float64(len(x.GetSamples())))
		}
		return
	}
}
