KERNEL := /lib/modules/$(shell uname -r)/build
TARGET := module

obj-m += $(TARGET).o
$(TARGET)-objs = module.o

all:
	make -C $(KERNEL) M=$(PWD) modules

clean:
	make -C $(KERNEL) M=$(PWD) clean
