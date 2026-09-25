interface Waiter<T> {
  resolve: (result: IteratorResult<T>) => void;
  reject: (err: Error) => void;
}

export class AsyncQueue<T> {
  private items: T[] = [];
  private waiter: Waiter<T> | null = null;
  private ended = false;
  private terminalError: Error | null = null;

  get size(): number {
    return this.items.length;
  }

  get endedState(): boolean {
    return this.ended;
  }

  push(item: T): void {
    if (this.ended) return;
    if (this.waiter) {
      const waiter = this.waiter;
      this.waiter = null;
      waiter.resolve({ value: item, done: false });
      return;
    }
    this.items.push(item);
  }

  removeFirst(predicate: (item: T) => boolean): boolean {
    for (let i = 0; i < this.items.length; i++) {
      if (predicate(this.items[i]!)) {
        this.items.splice(i, 1);
        return true;
      }
    }
    return false;
  }

  end(err?: Error): void {
    if (this.ended) return;
    this.ended = true;
    this.terminalError = err ?? null;
    if (this.waiter) {
      const waiter = this.waiter;
      this.waiter = null;
      if (this.terminalError) waiter.reject(this.terminalError);
      else waiter.resolve({ value: undefined as never, done: true });
    }
  }

  async *[Symbol.asyncIterator](): AsyncGenerator<T, void, undefined> {
    for (;;) {
      if (this.items.length > 0) {
        yield this.items.shift()!;
        continue;
      }
      if (this.ended) {
        if (this.terminalError) throw this.terminalError;
        return;
      }
      const result = await new Promise<IteratorResult<T>>((resolve, reject) => {
        this.waiter = { resolve, reject };
      });
      if (result.done) return;
      yield result.value;
    }
  }
}
