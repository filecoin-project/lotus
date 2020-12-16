#ifndef SRC_THREAD_POOL_HPP_
#define SRC_THREAD_POOL_HPP_

#include <vector>
#include <deque>
#include <functional>
#include <thread>
#include <condition_variable>
#include <mutex>
#include <atomic>

typedef std::function<void()> ThreadFunction;

class ThreadPool;

// Work object for the threads
class ThreadWork {
 protected:
  ThreadPool *pool;
  
 public:
  explicit ThreadWork(ThreadPool *_pool) {
    pool = _pool;
  }

  void run();
};

// Thread pool 
class ThreadPool {
 protected:
  friend class ThreadWork;

  // Cause threads to exit
  bool done;

  // Number of threads
  size_t num_threads;
  
  // Work
  std::deque<ThreadFunction> work;
  
  // Threads
  std::vector<std::thread> threads;

  // Thread synchronization
  std::mutex mtx;
  std::condition_variable ctvar;

 public:
  // Create a thread pool with size threads.
  ThreadPool(size_t _size) {
    done = false;
    num_threads = _size;

    for (size_t i = 0; i < num_threads; ++i) {
      threads.push_back(std::thread([this]() {
        ThreadWork(this).run();
      }));
    }
  }

  virtual ~ThreadPool() {
    done = true;
    ctvar.notify_all();
    for (size_t i = 0; i < num_threads; ++i) {
      threads[i].join();
    }
  }

  size_t size() {
    return(num_threads);
  }

  template<class W>
  void schedule(W w) {
    // Add the new work to the queue
    {
      std::unique_lock<std::mutex> lock(mtx);
      work.push_back(ThreadFunction(w));
    }
    // Wake up a thread to run
    ctvar.notify_one();
  }

  // The concept behind this function is similar to rust par_map. It will create
  // threads to process index-able work, such as in a vector or array. Func should
  // be a function that takes as an argument the index of the item to work on, for
  // example:
  //   void mywork(size_t idx)
  // Then this function will concurrently call mywork once for each idx using the
  // thread pool.
  //   num_items   - number of items to do work on
  //   max_threads - maximum number of threads to use. 0 means all available.
  template<class W>
  void parMap(size_t num_items, W func, size_t max_threads = 0) {
    std::atomic<size_t> work(0);
    size_t num_threads = std::min(size(), num_items);
    if (max_threads > 0) {
      num_threads = std::min(num_threads, max_threads);
    }
    
    std::vector<std::mutex> thread_complete(num_threads);
    for (size_t tid = 0; tid < num_threads; tid++) {
      thread_complete[tid].lock();
      schedule([tid, &func, &work, &thread_complete, num_items]() {
        while(true) {
          size_t i = work++;
          if (i >= num_items) {
            break;
          }
          func(i);
        }
        thread_complete[tid].unlock();
      });
    }
    // Gather the threads
    for (size_t tid = 0; tid < num_threads; tid++) {
      thread_complete[tid].lock();
    }
  }
};


#endif  // SRC_THREAD_POOL_HPP_
