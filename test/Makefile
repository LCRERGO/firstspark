CC ?= cc
CFLAGS ?= -O0 -g -fno-omit-frame-pointer

.PHONY: all clean

all: target

target: target.c
	$(CC) $(CFLAGS) -o target target.c

clean:
	rm -f target
