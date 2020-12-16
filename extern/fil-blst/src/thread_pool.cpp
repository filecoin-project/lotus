#include "thread_pool.hpp"

void ThreadWork::run() {
  while (1) {
    ThreadFunction work;
    {
      std::unique_lock<std::mutex> lock(pool->mtx);
      // Wait for some work
      while (!pool->done && pool->work.empty()) {
        pool->ctvar.wait(lock);
      }
      if(pool->done) {
        return;
      }

      work = pool->work.front();
      pool->work.pop_front();
    }
    // Call the function
    work();
  }
}
