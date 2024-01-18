package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	utils "gopfm4/pkg/utils"
)

type PodMeta struct {
	Pod       *corev1.Pod
	CgroupDir string
}

var (
	EventsMap = map[string][]string{
		"CPICollector": {"cycles", "instructions", "LONGEST_LAT_CACHE.MISS"},
	}
	collectors sync.Map
	//collectors = sync.Map{}
	CPUNUM int
)

func init() {
	CPUNUM = runtime.NumCPU()
	// CPUNUM = 1
}

func getContainerCgroupFile(podCgroupDir string, c *corev1.ContainerStatus) (*os.File, error) {
	containerCgroupFilePath, err := utils.CGroupContainerPath(podCgroupDir, c)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(containerCgroupFilePath, os.O_RDONLY, os.ModeDir)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func GetContainerPerfGroupCollector(podCgroupDir string, c *corev1.ContainerStatus, number int32, events []string) (*utils.PerfGroupCollector, error) {
	cpus := make([]int, number)
	for i := range cpus {
		cpus[i] = i
	}
	// get file descriptor for cgroup mode perf_event_open
	containerCgroupFile, err := getContainerCgroupFile(podCgroupDir, c)
	if err != nil {
		return nil, err
	}
	collector, err := utils.GetAndStartPerfGroupCollectorOnContainer(containerCgroupFile, cpus, events)
	if err != nil {
		return nil, err
	}
	return collector, nil
}

func getAndStartCollectorOnSingleContainer(podCgroupDir string, containerStatus *corev1.ContainerStatus, number int32, events []string) (*utils.PerfGroupCollector, error) {
	perfCollector, err := GetContainerPerfGroupCollector(podCgroupDir, containerStatus, number, events)
	if err != nil {
		klog.Errorf("get and start container %s collector err: %v", containerStatus.Name, err)
		return nil, err
	}
	return perfCollector, nil
}

func xcollectContainerCPI() {
	pods, err := utils.GetPods("b49691d6a544.jf.intel.com", "default")
	if err != nil {
		klog.Fatal(err)
	}
	klog.Infof("There are %d pods in the default namespace.", len(pods))
	// containerStatusesMap := map[*corev1.ContainerStatus]PodMeta{}
	collectors := sync.Map{}
	var wg sync.WaitGroup
	wg.Add(len(pods))

	for _, pod := range pods {
		for _, container := range pod.Status.ContainerStatuses {
			podCgroupDir, err := utils.CGroupDir(pod, &container)
			if err != nil {
				klog.Fatal(err)
			}
			if err != nil {
				klog.Fatal(err)
			}
			go func(status *corev1.ContainerStatus, podCgroupDir string) {
				defer wg.Done()
				collectorOnSingleContainer, err := getAndStartCollectorOnSingleContainer(podCgroupDir, status, int32(CPUNUM), EventsMap["CPICollector"])
				if err != nil {
					klog.Fatal(err)
				}
				collectors.Store(status.ContainerID, collectorOnSingleContainer)
			}(&container, podCgroupDir)
		}
	}
	wg.Wait()
	time.Sleep(10 * time.Second)
	var wg1 sync.WaitGroup
	// var mutex sync.Mutex
	wg1.Add(len(pods))
	for _, pod := range pods {
		for _, container := range pod.Status.ContainerStatuses {
			go func(status *corev1.ContainerStatus, pod *corev1.Pod) {
				defer wg1.Done()
				oneCollector, ok := collectors.Load(status.ContainerID)
				if ok {
					pc, _ := oneCollector.(*utils.PerfGroupCollector)
					cycles, instructions, cache_misses, err := utils.GetContainerCyclesAndInstructionsGroup(pc)
					if err != nil {
						klog.Fatal(err)
					}
					klog.Info("cache-misses: ", cache_misses, " cycles: ", cycles, " instructions: ", instructions, " pod: ", pod.Name)
				} else {
					klog.Info("collector not found ", err)
				}
			}(&container, pod)
		}
	}
}

func createCollectors() (pods []*corev1.Pod) {
	pods, err := utils.GetPods("b49691d6a544.jf.intel.com", "default")
	if err != nil {
		klog.Fatal(err)
	}
	klog.Infof("There are %d pods in the default namespace.", len(pods))
	var wg sync.WaitGroup
	wg.Add(len(pods))

	for _, pod := range pods {
		for _, container := range pod.Status.ContainerStatuses {
			podCgroupDir, err := utils.CGroupDir(pod, &container)
			if err != nil {
				klog.Fatal(err)
			}
			if err != nil {
				klog.Fatal(err)
			}
			go func(status *corev1.ContainerStatus, podCgroupDir string) {
				defer wg.Done()
				collectorOnSingleContainer, err := getAndStartCollectorOnSingleContainer(podCgroupDir, status, int32(CPUNUM), EventsMap["CPICollector"])
				if err != nil {
					klog.Fatal(err)
				}
				collectors.Store(status.ContainerID, collectorOnSingleContainer)
			}(&container, podCgroupDir)
		}
	}
	wg.Wait()
	return pods
}

// func collectContainerCPI(pods []*corev1.Pod) {
// 	var wg1 sync.WaitGroup
// 	klog.Infof("Begin to collect data for the %d pods in the default namespace.", len(pods))
// 	time.Sleep(1 * time.Second)
// 	wg1.Add(len(pods))
// 	for _, pod := range pods {
// 		for _, container := range pod.Status.ContainerStatuses {
// 			go func(status *corev1.ContainerStatus, pod *corev1.Pod) {
// 				defer wg1.Done()
// 				oneCollector, ok := collectors.Load(status.ContainerID)
// 				if ok {
// 					pc, _ := oneCollector.(*utils.PerfGroupCollector)
// 					cycles, instructions, cache_misses, err := utils.GetContainerCyclesAndInstructionsGroup(pc)
// 					//utils.CloseAllFds(pc)
// 					if err != nil {
// 						klog.Fatal(err)
// 					}
// 					klog.Info("cache-misses: ", cache_misses, " cycles: ", cycles, " instructions: ", instructions, " pod: ", pod.Name)
// 				} else {
// 					klog.Info("collector not found ")
// 				}
// 			}(&container, pod)
// 		}
// 	}
// }

func main() {
	//runtime.MemProfileRate = 128000
	// get current memory profile rate
	//klog.Info("MemProfileRate: ", runtime.MemProfileRate)
	// get current cpu profile rate
	// runtime.SetCPUProfileRate(128000)
	// disable memory profile
	runtime.MemProfileRate = 0
	runtime.SetMutexProfileFraction(0)
	runtime.SetBlockProfileRate(0)
	debug.SetGCPercent(10000)

	// beforeMaxProcs := runtime.GOMAXPROCS(0)
	// runtime.GOMAXPROCS(20)
	// aftermaxProcs := runtime.GOMAXPROCS(0)
	// klog.Info("BeforeMaxProcs: ", beforeMaxProcs, " AfterMaxProcs: ", aftermaxProcs)

	//runtime.GOMAXPROCS(50)
	var wg sync.WaitGroup
	wg.Add(1)
	ctx := utils.SetUpContext()

	utils.SetKubeConfig("/home/sdp/.kube/config")
	//stopCtx := signals.SetupSignalHandler()
	// Init library
	utils.LibInit()

	// Init events map
	eventsMap := make(map[int]struct{})
	eventsMap[len(EventsMap[string("CPICollector")])] = struct{}{}

	// Init buffer pool
	utils.InitBufferPool(eventsMap)

	go func() {
		<-ctx.Done()
		wg.Done()
		utils.LibFinalize()
	}()

	go func() {
		// 启动一个 http server，注意 pprof 相关的 handler 已经自动注册过了
		if err := http.ListenAndServe(":6060", nil); err != nil {
			klog.Fatal(err)
		}
	}()
	// pods := createCollectors()

	go wait.Until(func() {
		xcollectContainerCPI()
	}, 5*time.Second, ctx.Done())
	wg.Wait()
}
