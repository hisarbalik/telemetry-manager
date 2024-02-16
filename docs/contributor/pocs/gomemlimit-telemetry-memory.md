# GOMEMLIMIT for Telemetry Components
The Go version 1.19 introduces a new feature called GOMEMLIMIT, which can help us both increase GC-related performance and avoid GC-related out-of-memory (OOM) situations.

## Why Would You Run OOM?
There are two ways to allocate memory: on the stack or on the heap. A stack allocation is short-lived and typically very cheap. No Garbage Collection (GC) is required for stack allocation since the end of the function marks the end of the variable's lifetime. On the other hand, a heap allocation is long-lived and considerably more expensive. When allocating on the heap, the runtime must find a contiguous piece of memory where the new variable fits. Additionally, it must be garbage-collected when the variable is no longer used. Both operations are orders of magnitude more expensive than a stack allocation.

Short-lived allocations end on the stack, and long-lived allocations end up on the heap. In reality, it's not always this simple. Sometimes you will end up with unintentional heap allocations. It's important to know because those allocations will put pressure on the GC, which is required for preventing unexpected OOM situations.

Long-lived memory is something you can estimate upfront or control at runtime. For example, if you have a full-blown cache application, you likely have some sort of limit. Either the cache would stop accepting new values when it's full or start dropping old cache entries. For instance, you could ensure that the cache never exceeds 2GB in size. Then you should be safe on your 4GB machine. The answer is "maybe", but "maybe" is not enough when the risk is running out of memory.

To understand why it is possible to encounter OOM in this situation, we need to look at when the garbage collector runs. We know that we have 2GB of live memory, and simply by using the application, we add a few short-lived heap allocations here and there. We don't expect them to stick around long-term, but since there is no GC cycle running at the moment, they will never be freed. Eventually, we will encounter OOM when intentionally and unintentionally live heap exceeds 4GB.

Now let's look at the other extreme: the Garbage Collector runs extremely frequently. Any time our heap reaches 2.1GB, it runs and removes the 100MB of temporary allocation. An OOM situation is improbable now, but we have far exceeded our cost target; the application might now spend 30-40%, maybe more, on GC. This is no longer efficient.

The optimal solution is the best of two worlds: to get as close to our limit as possible but never beyond it. This way we can delay GC cycles until they are necessary. This will make our application fast, but at the same time, we can be sure that it never crosses the threshold, which makes our application safe from being OOM-killed.

### Go GC Targets
We want to make sure we use memory we have without going above it. Before Go 1.19, you had only one knob to turn: the GOGC environment variable. This environment variable accepts a relative target compared to the current live heap size. The default value for GOGC is 100, meaning that the heap should double before GC should run again.

That works well for applications that have small permanent live heaps. For example, if your constant heap is just 50MB and you have 4GB available, you can double your heap targets any time before ever being in danger. If the application load increases and temporary allocation increases, the dynamic targets would be 100MB, 200MB, 400MB, 800MB, 1600MB, and 3200MB. The load must double seven times to cross the 4GB mark, making running out of memory extremely unlikely.

But now think back to our cache application example with a permanent 2GB live heap on a 4GB machine. Even the first doubling of the heap is highly problematic because the new target (4GB) would already reach the limit of the physical memory on the machine.

Before Go 1.19, there was not much we could do about this; GOGC was the only knob that we could turn. So we most likely picked a value such as GOGC=25. That means the heap could grow by 25% before GC kicks in. Our new target would now be 2.5GB; unless the load changes drastically, we should be safe from running OOM.

This will work only at a single snapshot in time and assume that we always start with a 2GB live heap. But what if fewer items are in the cache, and the live heaps are only 100 MB? That would make our heap's goal just 125MB. In other words, we would end up with constant GC cycles, and they would take up a lot of CPU time.

Be Less Aggressive When Enough Memory Is Available, Be Very Aggressive When Less Memory Is Available
What we want to achieve is a situation where the GC is not very aggressive when a lot of memory is still available, but at the same time, the GC should become very aggressive when available free memory is tight. In the past, this was only possible with a workaround, the so-called "memory ballast" method. At the application startup, you would allocate a ballast, mostly a byte array that would take up a vast amount of memory, so you can make GOGC quite aggressive. Back to our example above, if you allocate a 2GB ballast and set GOGC=25, the GC will not run until 2.5GB memory is allocated.

## GOMEMLIMIT
While using virtual memory as a ballast improves the situation, it is still a workaround. With Go 1.19, we finally got a better solution: the GOMEMLIMIT allows specifying a soft memory cap. It does not replace GOGC but works in combination. We can set GOGC with a scenario in which memory is always available, and at the same time, we can trust that GOMEMLIMIT automatically makes the GC more aggressive when necessary.

When the live heap is low, e.g., 100MB, we can delay the next GC cycle until the heap has doubled, but when the heap has grown close to the limit, the GC runs more often to prevent us from encountering OOM.

### Soft Limit
The Go docs explicitly describe GOMEMLIMIT as a "soft" limit, which means the Go runtime does not guarantee that memory usage will exceed the limit; instead, it uses it as a target. The goal is to fail fast in an impossible-to-solve situation. Let's assume we set the limit to a value just a few kilobytes larger than the live heap; the GC will have to run constantly. We would be in a situation where the regular and GC execution would compete for the same resources, the application would stall, and since there is no way out other than running with more memory, the Go runtime prefers an OOM situation.

All the usable memory has been used up, and nothing can be freed anymore. That is a failure scenario, and fast failure is preferred. That makes the limit a "soft" limit.

## Test with TracePipeline
The test goal is to care about OOM safety as well as the throughput performance of TracePipeline. TracePipeline is a memory-intensive application and is a perfect candidate to benefit from GOMEMLIMIT.

For this experiment, we will use TracePipeline with OpenTelemetry Collector version 0.92. We will load TracePipeline with a huge amount of traces, observe GC behavior and memory usage during the test, and see when we run into OOM. In all scenarios, the same test data is used.

### Without GOMEMLIMIT
For the first run, we won't use GOMEMLIMIT. GOGC is set to 100, and available memory is 2GiB.

![TracePipeline without GOMEMLIMIT](./assets/without-gomemlimit.jpg)

As we can see from the memory snapshot above, the application starts with around ~980MiB live heap, and the next GC target is ~1.88GiB. After the GC cycle, the new target from 1.88GiB is ~3.7GiB, which is already above available memory, and the application will run into OOM.

### With GOMEMLIMIT
For the second run, we use GOMEMLIMIT with the value of 1.8GiB, GOGC is set to 100, and available memory is 2GiB.

![TracePipeline with GOMEMLIMIT](./assets/with-gomemlimit.jpg)

As we can see, the situation changed dramatically. There are no GC cycles until we reach our soft limit of 1.8GiB. Also, the GC was less aggressive, but after we got closer to our soft limit of 1.8GiB, the GC became more aggressive and ran often to recover memory.

In summary, GOMEMLIMIT made the GC more aggressive when less memory was available. The memory usage does not exceed our soft limit of 1.8GiB.

### Conclusion
- With our experiments, we can prove that our TracePipeline could crash on a 2GiB Pod with a load test, even when a constant load is less than 2GiB.
- After using GOMEMLIMIT=1.8GiB, TracePipeline no longer crashed and could efficiently use the available memory.
- Before Go 1.19, the Go runtime could only set relative GC targets. That would make it very hard to use the available memory efficiently.

Does that mean that GOMEMLIMIT is safe to avoid OOM? No, a Go application that gets heavy usage still has to ensure allocation efficiency. Simply setting a GOMEMLIMIT will not guarantee OOM will not happen. As we explained above, GOMEMLIMIT is a soft limit, and there is no guarantee the application will stay within the limit. The memory snapshot below shows exactly this situation. The TracePipeline is configured with GOMEMLIMIT of 1.8GiB, but the application gets much more load after reaching the configured limit. In this situation, the Go runtime will try to keep the application within the limits for a while, but when there is no other option than allocating more memory, the application will run into OOM.

![TracePipeline with GOMEMLIMIT and OOM](./assets/with-gomemlimit-oom.jpg)