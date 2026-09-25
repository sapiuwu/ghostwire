export class AsyncQueue {
    items = [];
    waiter = null;
    ended = false;
    terminalError = null;
    get size() {
        return this.items.length;
    }
    get endedState() {
        return this.ended;
    }
    push(item) {
        if (this.ended)
            return;
        if (this.waiter) {
            const waiter = this.waiter;
            this.waiter = null;
            waiter.resolve({ value: item, done: false });
            return;
        }
        this.items.push(item);
    }
    removeFirst(predicate) {
        for (let i = 0; i < this.items.length; i++) {
            if (predicate(this.items[i])) {
                this.items.splice(i, 1);
                return true;
            }
        }
        return false;
    }
    end(err) {
        if (this.ended)
            return;
        this.ended = true;
        this.terminalError = err ?? null;
        if (this.waiter) {
            const waiter = this.waiter;
            this.waiter = null;
            if (this.terminalError)
                waiter.reject(this.terminalError);
            else
                waiter.resolve({ value: undefined, done: true });
        }
    }
    async *[Symbol.asyncIterator]() {
        for (;;) {
            if (this.items.length > 0) {
                yield this.items.shift();
                continue;
            }
            if (this.ended) {
                if (this.terminalError)
                    throw this.terminalError;
                return;
            }
            const result = await new Promise((resolve, reject) => {
                this.waiter = { resolve, reject };
            });
            if (result.done)
                return;
            yield result.value;
        }
    }
}
//# sourceMappingURL=async-queue.js.map