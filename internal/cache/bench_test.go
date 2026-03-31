package cache

import (
	"fmt"
	"testing"
)

// BenchmarkCacheDB_UpsertProject benchmarks inserting/updating projects.
func BenchmarkCacheDB_UpsertProject(b *testing.B) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.UpsertProject(&Project{
			ID:        fmt.Sprintf("github.com/org/repo-%d", i%100),
			Ecosystem: "go",
			Name:      fmt.Sprintf("repo-%d", i%100),
		})
	}
}

// BenchmarkCacheDB_GetProject benchmarks project lookups.
func BenchmarkCacheDB_GetProject(b *testing.B) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Pre-populate
	for i := 0; i < 100; i++ {
		_ = db.UpsertProject(&Project{
			ID:        fmt.Sprintf("github.com/org/repo-%d", i),
			Ecosystem: "go",
			Name:      fmt.Sprintf("repo-%d", i),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = db.GetProject(fmt.Sprintf("github.com/org/repo-%d", i%100))
	}
}

// BenchmarkCacheDB_UpsertVersion benchmarks version insertion.
func BenchmarkCacheDB_UpsertVersion(b *testing.B) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	_ = db.UpsertProject(&Project{ID: "go/testpkg", Ecosystem: "go", Name: "testpkg"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.UpsertVersion(&ProjectVersion{
			ProjectID:  "go/testpkg",
			VersionKey: fmt.Sprintf("go/testpkg@1.0.%d", i%1000),
		})
	}
}

// BenchmarkCacheDB_AddVersionDependency benchmarks dependency edge insertion.
func BenchmarkCacheDB_AddVersionDependency(b *testing.B) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	_ = db.UpsertProject(&Project{ID: "go/parent", Ecosystem: "go", Name: "parent"})
	_ = db.UpsertVersion(&ProjectVersion{ProjectID: "go/parent", VersionKey: "go/parent@1.0.0"})

	for i := 0; i < 100; i++ {
		pid := fmt.Sprintf("go/child-%d", i)
		_ = db.UpsertProject(&Project{ID: pid, Ecosystem: "go", Name: fmt.Sprintf("child-%d", i)})
		_ = db.UpsertVersion(&ProjectVersion{ProjectID: pid, VersionKey: fmt.Sprintf("%s@1.0.0", pid)})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		childID := fmt.Sprintf("go/child-%d", i%100)
		_ = db.AddVersionDependency(&VersionDependency{
			ParentProjectID: "go/parent", ParentVersionKey: "go/parent@1.0.0",
			ChildProjectID: childID, ChildVersionConstraint: childID + "@1.0.0", DepScope: "depends_on",
		})
	}
}

// BenchmarkCacheDB_GetVersionDependencies benchmarks dependency lookups.
func BenchmarkCacheDB_GetVersionDependencies(b *testing.B) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	_ = db.UpsertProject(&Project{ID: "go/parent", Ecosystem: "go", Name: "parent"})
	_ = db.UpsertVersion(&ProjectVersion{ProjectID: "go/parent", VersionKey: "go/parent@1.0.0"})

	for i := 0; i < 50; i++ {
		pid := fmt.Sprintf("go/child-%d", i)
		_ = db.UpsertProject(&Project{ID: pid, Ecosystem: "go", Name: fmt.Sprintf("child-%d", i)})
		_ = db.UpsertVersion(&ProjectVersion{ProjectID: pid, VersionKey: fmt.Sprintf("%s@1.0.0", pid)})
		_ = db.AddVersionDependency(&VersionDependency{
			ParentProjectID: "go/parent", ParentVersionKey: "go/parent@1.0.0",
			ChildProjectID: pid, ChildVersionConstraint: pid + "@1.0.0", DepScope: "depends_on",
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = db.GetVersionDependencies("go/parent", "go/parent@1.0.0")
	}
}

// BenchmarkCacheDB_FindDependents benchmarks reverse dependency lookups.
func BenchmarkCacheDB_FindDependents(b *testing.B) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	_ = db.UpsertProject(&Project{ID: "go/shared", Ecosystem: "go", Name: "shared"})
	_ = db.UpsertVersion(&ProjectVersion{ProjectID: "go/shared", VersionKey: "go/shared@1.0.0"})

	// 50 parents all depend on the same shared package
	for i := 0; i < 50; i++ {
		pid := fmt.Sprintf("go/parent-%d", i)
		_ = db.UpsertProject(&Project{ID: pid, Ecosystem: "go", Name: fmt.Sprintf("parent-%d", i)})
		_ = db.UpsertVersion(&ProjectVersion{ProjectID: pid, VersionKey: fmt.Sprintf("%s@1.0.0", pid)})
		_ = db.AddVersionDependency(&VersionDependency{
			ParentProjectID: pid, ParentVersionKey: pid + "@1.0.0",
			ChildProjectID: "go/shared", ChildVersionConstraint: "go/shared@1.0.0", DepScope: "depends_on",
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = db.FindDependents("go/shared")
	}
}

// BenchmarkCacheDB_RefResolution benchmarks ref resolution cache hits.
func BenchmarkCacheDB_RefResolution(b *testing.B) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	_ = db.SetRefResolution("actions/checkout", "v4", "abc123", "tag")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = db.GetRefResolution("actions/checkout", "v4", "tag")
	}
}
