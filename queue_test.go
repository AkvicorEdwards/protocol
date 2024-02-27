package protocol

import (
	"fmt"
	"testing"
	"time"
)

func TestQueue(t *testing.T) {
	pkg1 := newPackage(0, 0, 1, []byte{1})
	pkg2 := newPackage(0, 0, 2, []byte{1, 2})

	que := newQueue(0)
	if que.size() != 0 || !que.isEmpty() {
		t.Errorf("size:%d isEmpty:%v\n", que.size(), que.isEmpty())
	}

	que.push(pkg1, 0)
	if que.size() != 1 || que.isEmpty() {
		t.Errorf("size:%d isEmpty:%v\n", que.size(), que.isEmpty())
	}

	pkg11 := que.pop(0)
	if que.size() != 0 || !que.isEmpty() {
		t.Errorf("size:%d isEmpty:%v\n", que.size(), que.isEmpty())
	}
	if pkg11.value != 1 || len(pkg11.data) != 1 || pkg11.data[0] != 1 {
		t.Errorf("value:%d data:%v\n", pkg11.value, pkg11.data)
	}
	que.push(pkg2, 0)
	if que.size() != 1 || que.isEmpty() {
		t.Errorf("size:%d isEmpty:%v\n", que.size(), que.isEmpty())
	}

	pkg21 := que.pop(0)
	if pkg21.value != 2 || len(pkg21.data) != 2 || pkg21.data[0] != 1 || pkg21.data[1] != 2 {
		t.Errorf("value:%d data:%v\n", pkg21.value, pkg21.data)
	}

	pkg1 = newPackage(0, 0, 1, nil)
	pkg2 = newPackage(0, 0, 2, nil)
	pkg3 := newPackage(0, 0, 3, nil)
	pkg4 := newPackage(0, 0, 4, nil)
	pkg5 := newPackage(0, 0, 5, []byte{55})

	que2 := newQueue(3)
	if que2.size() != 0 {
		t.Errorf("size:%d isEmpty:%v\n", que.size(), que.isEmpty())
	}

	que2.push(pkg1, 0)
	if que2.size() != 1 {
		t.Errorf("size:%d isEmpty:%v\n", que.size(), que.isEmpty())
	}

	que2.push(pkg2, 0)
	if que2.size() != 2 {
		t.Errorf("size:%d isEmpty:%v\n", que.size(), que.isEmpty())
	}

	que2.push(pkg3, 0)
	if que2.size() != 3 {
		t.Errorf("size:%d isEmpty:%v\n", que.size(), que.isEmpty())
	}

	go func() {
		time.Sleep(3 * time.Second)
		fmt.Println("pop")
		pkg11 := que2.pop(0)
		if pkg11.value != 1 || pkg11.data != nil {
			t.Errorf("value:%d data:%v\n", pkg11.value, pkg11.data)
		}
	}()

	fmt.Println("wait pop")
	que2.push(pkg4, 0)
	if que2.size() != 3 {
		t.Errorf("size:%d isEmpty:%v\n", que.size(), que.isEmpty())
	}

	pkg21 = que2.pop(0)
	if pkg21.value != 2 || pkg21.data != nil {
		t.Errorf("value:%d data:%v\n", pkg21.value, pkg21.data)
	}

	pkg31 := que2.pop(0)
	if pkg31.value != 3 || pkg31.data != nil {
		t.Errorf("value:%d data:%v\n", pkg31.value, pkg31.data)
	}

	pkg41 := que2.pop(0)
	if pkg41.value != 4 || pkg31.data != nil {
		t.Errorf("value:%d data:%v\n", pkg41.value, pkg41.data)
	}

	go func() {
		time.Sleep(3 * time.Second)
		fmt.Println("push")
		que2.push(pkg5, 0)
	}()

	fmt.Println("wait push")
	pkg51 := que2.pop(0)
	if pkg51.value != 5 || len(pkg51.data) != 1 || pkg51.data[0] != 55 {
		t.Errorf("value:%d data:%v\n", pkg51.value, pkg51.data)
	}

	fmt.Println("size")
	if que2.size() != 0 {
		t.Errorf("size:%d isEmpty:%v\n", que.size(), que.isEmpty())
	}
}
