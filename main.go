package main

func main() {
	worker := &FlowWorker{}
	worker.Synchronize("http://karaf:karaf@127.0.0.1:8181", "of:00000800276f723f")
}
