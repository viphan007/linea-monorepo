/*
 * Copyright Consensys Software Inc.
 *
 * This file is dual-licensed under either the MIT license or Apache License 2.0.
 * See the LICENSE-MIT and LICENSE-APACHE files in the repository root for details.
 *
 * SPDX-License-Identifier: MIT OR Apache-2.0
 */
package net.consensys.linea.utils;

import java.util.concurrent.Callable;
import java.util.concurrent.FutureTask;
import java.util.concurrent.PriorityBlockingQueue;
import java.util.concurrent.RunnableFuture;
import java.util.concurrent.ThreadFactory;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import lombok.EqualsAndHashCode;
import lombok.Getter;

public class PriorityThreadPoolExecutor extends ThreadPoolExecutor {
  public PriorityThreadPoolExecutor(
      final int corePoolSize,
      final int maximumPoolSize,
      final long keepAliveTime,
      final TimeUnit unit,
      final ThreadFactory threadFactory) {
    super(
        corePoolSize,
        maximumPoolSize,
        keepAliveTime,
        unit,
        new PriorityBlockingQueue<>(),
        threadFactory);
  }

  @Override
  protected <T> RunnableFuture<T> newTaskFor(final Runnable runnable, final T value) {
    return new PriorityFuture<>(runnable, value);
  }

  @Override
  protected <T> RunnableFuture<T> newTaskFor(final Callable<T> callable) {
    return new PriorityFuture<>(callable);
  }

  public <T> boolean remove(final Callable<T> callable) {
    return super.remove(new PriorityFuture<>(callable));
  }

  public boolean remove(final Runnable runnable) {
    return super.remove(new PriorityFuture<>(runnable, null));
  }

  // we delegate equality to source class so the remove works
  @EqualsAndHashCode(callSuper = false, onlyExplicitlyIncluded = true)
  public static class PriorityFuture<T> extends FutureTask<T>
      implements Comparable<PriorityFuture<T>> {
    @Getter @EqualsAndHashCode.Include private Object sourceTask;

    public PriorityFuture(final Runnable runnable, final T result) {
      super(runnable, result);
      sourceTask = runnable;
    }

    public PriorityFuture(final Callable<T> callable) {
      super(callable);
      sourceTask = callable;
    }

    @Override
    @SuppressWarnings("unchecked")
    public int compareTo(PriorityFuture<T> o) {
      return ((Comparable) sourceTask).compareTo(o.sourceTask);
    }
  }
}
