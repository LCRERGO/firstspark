/* A tiny fixture process with known values in memory. Build with `make` and
 * attach Firstspark to it while it runs. */
#include <stdio.h>
#include <unistd.h>

volatile int health = 1000;
volatile int gold = 250;
volatile double mana = 3.5;
volatile char name[16] = "firstspark";

int main(void) {
    printf("pid=%d health=%p gold=%p mana=%p name=%p\n",
           (int)getpid(), (void *)&health, (void *)&gold,
           (void *)&mana, (void *)name);
    fflush(stdout);
    for (;;) {
        health += 1;
        usleep(100000);
    }
    return 0;
}
