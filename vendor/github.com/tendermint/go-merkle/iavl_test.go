package merkle

import (
	"bytes"
	"fmt"

	. "github.com/tendermint/go-common"
	. "github.com/tendermint/go-common/test"
	"github.com/tendermint/go-db"
	"github.com/tendermint/go-wire"

	"runtime"
	"testing"
)

const testReadLimit = 1 << 20 // Some reasonable limit for wire.Read*() lmt

func randstr(length int) string {
	return RandStr(length)
}

func i2b(i int) []byte {
	bz := make([]byte, 4)
	wire.PutInt32(bz, int32(i))
	return bz
}

func b2i(bz []byte) int {
	i := wire.GetInt32(bz)
	return int(i)
}

// Convenience for a new node
func N(l, r interface{}) *IAVLNode {
	var left, right *IAVLNode
	if _, ok := l.(*IAVLNode); ok {
		left = l.(*IAVLNode)
	} else {
		left = NewIAVLNode(i2b(l.(int)), nil)
	}
	if _, ok := r.(*IAVLNode); ok {
		right = r.(*IAVLNode)
	} else {
		right = NewIAVLNode(i2b(r.(int)), nil)
	}

	n := &IAVLNode{
		key:       right.lmd(nil).key,
		value:     nil,
		leftNode:  left,
		rightNode: right,
	}
	n.calcHeightAndSize(nil)
	return n
}

// Setup a deep node
func T(n *IAVLNode) *IAVLTree {
	t := NewIAVLTree(0, nil)
	n.hashWithCount(t)
	t.root = n
	return t
}

// Convenience for simple printing of keys & tree structure
func P(n *IAVLNode) string {
	if n.height == 0 {
		return fmt.Sprintf("%v", b2i(n.key))
	} else {
		return fmt.Sprintf("(%v %v)", P(n.leftNode), P(n.rightNode))
	}
}

func TestBasic(t *testing.T) {
	var tree *IAVLTree = NewIAVLTree(0, nil)
	var up bool
	up = tree.Set([]byte("1"), []byte("one"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	up = tree.Set([]byte("2"), []byte("two"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	up = tree.Set([]byte("2"), []byte("TWO"))
	if !up {
		t.Error("Expected an update")
	}
	up = tree.Set([]byte("5"), []byte("five"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}

	// Test 0x00
	{
		idx, val, exists := tree.Get([]byte{0x00})
		if exists {
			t.Errorf("Expected no value to exist")
		}
		if idx != 0 {
			t.Errorf("Unexpected idx %x", idx)
		}
		if string(val) != "" {
			t.Errorf("Unexpected value %v", string(val))
		}
	}

	// Test "1"
	{
		idx, val, exists := tree.Get([]byte("1"))
		if !exists {
			t.Errorf("Expected value to exist")
		}
		if idx != 0 {
			t.Errorf("Unexpected idx %x", idx)
		}
		if string(val) != "one" {
			t.Errorf("Unexpected value %v", string(val))
		}
	}

	// Test "2"
	{
		idx, val, exists := tree.Get([]byte("2"))
		if !exists {
			t.Errorf("Expected value to exist")
		}
		if idx != 1 {
			t.Errorf("Unexpected idx %x", idx)
		}
		if string(val) != "TWO" {
			t.Errorf("Unexpected value %v", string(val))
		}
	}

	// Test "4"
	{
		idx, val, exists := tree.Get([]byte("4"))
		if exists {
			t.Errorf("Expected no value to exist")
		}
		if idx != 2 {
			t.Errorf("Unexpected idx %x", idx)
		}
		if string(val) != "" {
			t.Errorf("Unexpected value %v", string(val))
		}
	}
}

func TestUnit(t *testing.T) {

	expectHash := func(tree *IAVLTree, hashCount int) {
		// ensure number of new hash calculations is as expected.
		hash, count := tree.HashWithCount()
		if count != hashCount {
			t.Fatalf("Expected %v new hashes, got %v", hashCount, count)
		}
		// nuke hashes and reconstruct hash, ensure it's the same.
		tree.root.traverse(tree, func(node *IAVLNode) bool {
			node.hash = nil
			return false
		})
		// ensure that the new hash after nuking is the same as the old.
		newHash, _ := tree.HashWithCount()
		if bytes.Compare(hash, newHash) != 0 {
			t.Fatalf("Expected hash %v but got %v after nuking", hash, newHash)
		}
	}

	expectSet := func(tree *IAVLTree, i int, repr string, hashCount int) {
		origNode := tree.root
		updated := tree.Set(i2b(i), nil)
		// ensure node was added & structure is as expected.
		if updated == true || P(tree.root) != repr {
			t.Fatalf("Adding %v to %v:\nExpected         %v\nUnexpectedly got %v updated:%v",
				i, P(origNode), repr, P(tree.root), updated)
		}
		// ensure hash calculation requirements
		expectHash(tree, hashCount)
		tree.root = origNode
	}

	expectRemove := func(tree *IAVLTree, i int, repr string, hashCount int) {
		origNode := tree.root
		value, removed := tree.Remove(i2b(i))
		// ensure node was added & structure is as expected.
		if len(value) != 0 || !removed || P(tree.root) != repr {
			t.Fatalf("Removing %v from %v:\nExpected         %v\nUnexpectedly got %v value:%v removed:%v",
				i, P(origNode), repr, P(tree.root), value, removed)
		}
		// ensure hash calculation requirements
		expectHash(tree, hashCount)
		tree.root = origNode
	}

	//////// Test Set cases:

	// Case 1:
	t1 := T(N(4, 20))

	expectSet(t1, 8, "((4 8) 20)", 3)
	expectSet(t1, 25, "(4 (20 25))", 3)

	t2 := T(N(4, N(20, 25)))

	expectSet(t2, 8, "((4 8) (20 25))", 3)
	expectSet(t2, 30, "((4 20) (25 30))", 4)

	t3 := T(N(N(1, 2), 6))

	expectSet(t3, 4, "((1 2) (4 6))", 4)
	expectSet(t3, 8, "((1 2) (6 8))", 3)

	t4 := T(N(N(1, 2), N(N(5, 6), N(7, 9))))

	expectSet(t4, 8, "(((1 2) (5 6)) ((7 8) 9))", 5)
	expectSet(t4, 10, "(((1 2) (5 6)) (7 (9 10)))", 5)

	//////// Test Remove cases:

	t10 := T(N(N(1, 2), 3))

	expectRemove(t10, 2, "(1 3)", 1)
	expectRemove(t10, 3, "(1 2)", 0)

	t11 := T(N(N(N(1, 2), 3), N(4, 5)))

	expectRemove(t11, 4, "((1 2) (3 5))", 2)
	expectRemove(t11, 3, "((1 2) (4 5))", 1)

}

func TestIntegration(t *testing.T) {

	type record struct {
		key   string
		value string
	}

	records := make([]*record, 400)
	var tree *IAVLTree = NewIAVLTree(0, nil)

	randomRecord := func() *record {
		return &record{randstr(20), randstr(20)}
	}

	for i := range records {
		r := randomRecord()
		records[i] = r
		//t.Log("New record", r)
		//PrintIAVLNode(tree.root)
		updated := tree.Set([]byte(r.key), nil)
		if updated {
			t.Error("should have not been updated")
		}
		updated = tree.Set([]byte(r.key), []byte(r.value))
		if !updated {
			t.Error("should have been updated")
		}
		if tree.Size() != i+1 {
			t.Error("size was wrong", tree.Size(), i+1)
		}
	}

	for _, r := range records {
		if has := tree.Has([]byte(r.key)); !has {
			t.Error("Missing key", r.key)
		}
		if has := tree.Has([]byte(randstr(12))); has {
			t.Error("Table has extra key")
		}
		if _, val, _ := tree.Get([]byte(r.key)); string(val) != string(r.value) {
			t.Error("wrong value")
		}
	}

	for i, x := range records {
		if val, removed := tree.Remove([]byte(x.key)); !removed {
			t.Error("Wasn't removed")
		} else if string(val) != string(x.value) {
			t.Error("Wrong value")
		}
		for _, r := range records[i+1:] {
			if has := tree.Has([]byte(r.key)); !has {
				t.Error("Missing key", r.key)
			}
			if has := tree.Has([]byte(randstr(12))); has {
				t.Error("Table has extra key")
			}
			_, val, _ := tree.Get([]byte(r.key))
			if string(val) != string(r.value) {
				t.Error("wrong value")
			}
		}
		if tree.Size() != len(records)-(i+1) {
			t.Error("size was wrong", tree.Size(), (len(records) - (i + 1)))
		}
	}
}

func TestPersistence(t *testing.T) {
	db := db.NewMemDB()

	// Create some random key value pairs
	records := make(map[string]string)
	for i := 0; i < 10000; i++ {
		records[randstr(20)] = randstr(20)
	}

	// Construct some tree and save it
	t1 := NewIAVLTree(0, db)
	for key, value := range records {
		t1.Set([]byte(key), []byte(value))
	}
	t1.Save()

	hash, _ := t1.HashWithCount()

	// Load a tree
	t2 := NewIAVLTree(0, db)
	t2.Load(hash)
	for key, value := range records {
		_, t2value, _ := t2.Get([]byte(key))
		if string(t2value) != value {
			t.Fatalf("Invalid value. Expected %v, got %v", value, t2value)
		}
	}
}

func testProof(t *testing.T, proof *IAVLProof, keyBytes, valueBytes, rootHash []byte) {
	// Proof must verify.
	if !proof.Verify(keyBytes, valueBytes, rootHash) {
		t.Errorf("Invalid proof. Verification failed.")
		return
	}
	// Write/Read then verify.
	proofBytes := wire.BinaryBytes(proof)
	n, err := int(0), error(nil)
	proof2 := wire.ReadBinary(&IAVLProof{}, bytes.NewBuffer(proofBytes), 0, &n, &err).(*IAVLProof)
	if err != nil {
		t.Errorf("Failed to read IAVLProof from bytes: %v", err)
		return
	}
	if !proof2.Verify(keyBytes, valueBytes, rootHash) {
		// t.Log(Fmt("%X\n%X\n", proofBytes, wire.BinaryBytes(proof2)))
		t.Errorf("Invalid proof after write/read. Verification failed.")
		return
	}
	// Random mutations must not verify
	for i := 0; i < 5; i++ {
		badProofBytes := MutateByteSlice(proofBytes)
		n, err := int(0), error(nil)
		badProof := wire.ReadBinary(&IAVLProof{}, bytes.NewBuffer(badProofBytes), testReadLimit, &n, &err).(*IAVLProof)
		if err != nil {
			continue // This is fine.
		}
		if badProof.Verify(keyBytes, valueBytes, rootHash) {
			t.Errorf("Proof was still valid after a random mutation:\n%X\n%X", proofBytes, badProofBytes)
		}
	}
}

func TestIAVLProof(t *testing.T) {

	// Construct some random tree
	db := db.NewMemDB()
	var tree *IAVLTree = NewIAVLTree(100, db)
	for i := 0; i < 1000; i++ {
		key, value := randstr(20), randstr(20)
		tree.Set([]byte(key), []byte(value))
	}

	// Persist the items so far
	tree.Save()

	// Add more items so it's not all persisted
	for i := 0; i < 100; i++ {
		key, value := randstr(20), randstr(20)
		tree.Set([]byte(key), []byte(value))
	}

	// Now for each item, construct a proof and verify
	tree.Iterate(func(key []byte, value []byte) bool {
		proof := tree.ConstructProof(key)
		if !bytes.Equal(proof.RootHash, tree.Hash()) {
			t.Errorf("Invalid proof. Expected root %X, got %X", tree.Hash(), proof.RootHash)
		}
		testProof(t, proof, key, value, tree.Hash())
		return false
	})

}

func BenchmarkImmutableAvlTreeLevelDB2(b *testing.B) {
	b.StopTimer()

	db := db.NewDB("test", "leveldb2", "./")
	t := NewIAVLTree(100000, db)
	// for i := 0; i < 10000000; i++ {
	for i := 0; i < 1000000; i++ {
		// for i := 0; i < 1000; i++ {
		t.Set(i2b(int(RandInt32())), nil)
		if i > 990000 && i%1000 == 999 {
			t.Save()
		}
	}
	t.Save()

	fmt.Println("ok, starting")

	runtime.GC()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ri := i2b(int(RandInt32()))
		t.Set(ri, nil)
		t.Remove(ri)
		if i%100 == 99 {
			t.Save()
		}
	}

	db.Close()
}

func BenchmarkImmutableAvlTreeMemDB(b *testing.B) {
	b.StopTimer()

	db := db.NewDB("test", "memdb", "")
	t := NewIAVLTree(100000, db)
	// for i := 0; i < 10000000; i++ {
	for i := 0; i < 1000000; i++ {
		// for i := 0; i < 1000; i++ {
		t.Set(i2b(int(RandInt32())), nil)
		if i > 990000 && i%1000 == 999 {
			t.Save()
		}
	}
	t.Save()

	fmt.Println("ok, starting")

	runtime.GC()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ri := i2b(int(RandInt32()))
		t.Set(ri, nil)
		t.Remove(ri)
		if i%100 == 99 {
			t.Save()
		}
	}

	db.Close()
}
