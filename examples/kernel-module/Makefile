# out-of-tree called make with two arguments -- kernel headers path and name
# of target binary that MUST be produced by makefile
# e.g.:
#    make KERNEL=/lib/modules/4.8.0-58-generic/build TARGET=hello-world.ko
KERNEL := /lib/modules/$(shell uname -r)/build
TARGET := out-of-tree-module-example

obj-m += $(TARGET).o
$(TARGET)-objs = module.o

all:
	make -C $(KERNEL) M=$(PWD) modules

clean:
	make -C $(KERNEL) M=$(PWD) clean
