package compiler

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/tinygo-org/tinygo/compiler/llvmutil"
	"github.com/tinygo-org/tinygo/goenv"
	"golang.org/x/tools/go/ssa"
	"tinygo.org/x/go-llvm"
)

// validateRegionsGo enforces the task boundary. Ordinary regions are bound to
// the creating task, so a goroutine may only receive values that cannot carry
// an automatic allocation. Region handles themselves are control objects and
// may be handed to another task for user-managed release.
func (b *builder) validateRegionsGo(instr *ssa.Go) error {
	var closureFn *ssa.Function
	if closure, ok := instr.Call.Value.(*ssa.MakeClosure); ok {
		closureFn, _ = closure.Fn.(*ssa.Function)
		if closureFn == nil {
			return b.makeError(instr.Pos(), "gc=regions requires a statically resolved goroutine function")
		}
	}
	for _, arg := range instr.Call.Args {
		if typeContainsReference(arg.Type()) && !isManualRegionHandle(arg.Type()) {
			if _, ok := b.regionManualOwnerForValue(arg); ok {
				continue
			}
			return b.makeError(instr.Pos(), fmt.Sprintf("gc=regions cannot pass automatic reference %s to a goroutine; use values or an explicit regions.Region", arg.Type()))
		}
	}
	if closureFn != nil {
		for _, freeVar := range closureFn.FreeVars {
			if typeContainsReference(freeVar.Type()) && !isManualRegionHandle(freeVar.Type()) {
				if _, ok := b.regionManualOwnerForValue(freeVar); ok {
					continue
				}
				return b.makeError(instr.Pos(), fmt.Sprintf("gc=regions goroutine closure captures automatic reference %s; use values or an explicit regions.Region", freeVar.Type()))
			}
		}
	}
	return nil
}

// regionsStrictPackage identifies source packages whose object lifetimes are
// part of the user-facing regions contract. The runtime and Go/TinyGo standard
// library use a small number of static tables and bootstrapping globals which
// are outside that contract, although their dynamic allocations still follow
// the active owner through runtime.alloc.
func (b *builder) regionsStrictPackage() bool {
	return b.isRegionsStrictFunction(b.fn)
}

func (c *compilerContext) isRegionsStrictFunction(fn *ssa.Function) bool {
	if fn == nil {
		return false
	}
	// x/sys is the hosted platform syscall adapter, just like the standard
	// syscall package. It necessarily round-trips OS addresses through uintptr
	// and uses generated callback tables; those values are external-owned, not
	// Go region allocations. Keep the strict user-object contract at its call
	// boundary instead of rejecting the adapter implementation itself.
	if fn.Pkg != nil && fn.Pkg.Pkg != nil && strings.HasPrefix(fn.Pkg.Pkg.Path(), "golang.org/x/sys/") {
		return false
	}
	return c.isRegionsStrictPosition(fn.Pos())
}

// isRegionsStrictPosition reports whether a declaration is user source rather
// than Go/TinyGo standard-library source. Interface invoke thunks use the
// normal ABI, so a user call may only dispatch methods declared by the latter
// (or by the predeclared universe) until owner-aware invoke thunks exist.
func (c *compilerContext) isRegionsStrictPosition(pos token.Pos) bool {
	position := c.program.Fset.Position(pos)
	filename := position.Filename
	if filename == "" {
		return false
	}
	filename = filepath.Clean(filename)
	for _, root := range []string{goenv.Get("GOROOT"), goenv.Get("TINYGOROOT")} {
		if root == "" {
			continue
		}
		stdlib := filepath.Clean(filepath.Join(root, "src")) + string(filepath.Separator)
		if strings.HasPrefix(filename, stdlib) {
			return false
		}
	}
	return true
}

// regionInterfaceInvokeAllowed reports whether an interface invoke can use the
// regions-aware thunk ABI. The thunk carries its selected owner in its final
// context slot, allowing it to call either a standard method or a user method
// with the private owner ABI.
func (b *builder) regionInterfaceInvokeAllowed(call *ssa.CallCommon) bool {
	return true
}

// regionFFINocaptureCall identifies an external C declaration. TinyGo marks
// its pointer parameters nocapture; regions additionally relies on the
// documented synchronous FFI contract and never treats a Go definition with
// //export as an imported function.
func (b *builder) regionFFINocaptureCall(call *ssa.CallCommon) bool {
	fn := call.StaticCallee()
	return fn != nil && fn.Blocks == nil && b.getFunctionInfo(fn).exported
}

// regionUnsafePointerToUintptrAllowed permits two constrained, non-owning
// address uses: comparing an external FFI pointer sentinel, and placing a Go
// address in a stack-resident ABI aggregate that is passed only to one or more
// synchronous nocapture FFI calls. It intentionally does not permit rebuilding
// a Go pointer from uintptr, returning it, storing it globally, or forwarding
// the aggregate through Go code.
func (b *builder) regionUnsafePointerToUintptrAllowed(convert *ssa.Convert) bool {
	refs := convert.Referrers()
	if refs == nil || len(*refs) == 0 {
		return false
	}
	for _, ref := range *refs {
		switch ref := ref.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.BinOp:
			// A comparison cannot extend the lifetime of the address. Restrict
			// this form to pointers returned by a direct external declaration.
			switch ref.Op {
			case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
				if !regionValueFromFFIResult(convert.X, nil, b) {
					return false
				}
			default:
				return false
			}
		case *ssa.Store:
			if ref.Val != convert || !regionStackAggregateOnlyPassedToFFI(ref.Addr, b, nil) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func regionValueFromFFIResult(value ssa.Value, seen map[ssa.Value]bool, b *builder) bool {
	if seen == nil {
		seen = make(map[ssa.Value]bool)
	}
	if seen[value] {
		return false
	}
	seen[value] = true
	switch value := value.(type) {
	case *ssa.Call:
		return b.regionFFINocaptureCall(value.Common())
	case *ssa.ChangeType:
		return regionValueFromFFIResult(value.X, seen, b)
	case *ssa.Phi:
		for _, edge := range value.Edges {
			if !regionValueFromFFIResult(edge, seen, b) {
				return false
			}
		}
		return len(value.Edges) != 0
	}
	return false
}

// regionStackAggregateOnlyPassedToFFI follows address projections back to a
// stack allocation, then verifies that every exposed address remains local or
// reaches a direct external declaration. This accepts C ABI structs such as
// iovec without making uintptr a generally escaping type.
func regionStackAggregateOnlyPassedToFFI(addr ssa.Value, b *builder, seen map[ssa.Value]bool) bool {
	root := regionStackAggregateRoot(addr)
	if root == nil {
		return false
	}
	if seen == nil {
		seen = make(map[ssa.Value]bool)
	}
	return regionAddressUsesOnlyFFI(root, b, seen)
}

func regionStackAggregateRoot(value ssa.Value) *ssa.Alloc {
	switch value := value.(type) {
	case *ssa.Alloc:
		return value
	case *ssa.FieldAddr:
		return regionStackAggregateRoot(value.X)
	case *ssa.IndexAddr:
		return regionStackAggregateRoot(value.X)
	case *ssa.ChangeType:
		return regionStackAggregateRoot(value.X)
	}
	return nil
}

func regionAddressUsesOnlyFFI(value ssa.Value, b *builder, seen map[ssa.Value]bool) bool {
	if seen[value] {
		return true
	}
	seen[value] = true
	refs := value.Referrers()
	if refs == nil {
		return false
	}
	for _, ref := range *refs {
		switch ref := ref.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.FieldAddr:
			if !regionAddressUsesOnlyFFI(ref, b, seen) {
				return false
			}
		case *ssa.IndexAddr:
			if !regionAddressUsesOnlyFFI(ref, b, seen) {
				return false
			}
		case *ssa.ChangeType:
			if !regionAddressUsesOnlyFFI(ref, b, seen) {
				return false
			}
		case *ssa.Store:
			if ref.Addr != value {
				return false
			}
		case *ssa.UnOp:
			// Composite literals are commonly materialized in a local value and
			// copied into the ABI aggregate. Permit that load only when every
			// resulting value is stored in another aggregate with the same FFI-
			// only address contract.
			if ref.Op != token.MUL || !regionAggregateLoadOnlyPassedToFFI(ref, b, seen) {
				return false
			}
		case *ssa.Call:
			if !b.regionFFINocaptureCall(ref.Common()) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func regionAggregateLoadOnlyPassedToFFI(load *ssa.UnOp, b *builder, seen map[ssa.Value]bool) bool {
	refs := load.Referrers()
	if refs == nil || len(*refs) == 0 {
		return false
	}
	for _, ref := range *refs {
		switch ref := ref.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.Store:
			if ref.Val != load || !regionStackAggregateOnlyPassedToFFI(ref.Addr, b, seen) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// hasRegionOwnerABI identifies the subset of Go functions whose private ABI
// carries an explicit allocation owner. Keep public/runtime entry points and
// function literals on the normal ABI: the latter may be called through a
// func value by code that cannot statically know the owner extension.
func (c *compilerContext) hasRegionOwnerABI(fn *ssa.Function) bool {
	if !c.Regions || fn == nil || fn.Parent() != nil {
		return false
	}
	// A method wrapper is entered through a statically known method set and
	// immediately forwards to its declared method. In particular, Go creates
	// one when *T satisfies an interface through a value-receiver method on T.
	// It must carry the owner so the forwarding call does not fall back to the
	// ambient Region. Keep closures and bound method wrappers on their normal
	// ABI because those can be called through an ordinary func value.
	isMethodWrapper := strings.HasPrefix(fn.Synthetic, "wrapper for ")
	if fn.Synthetic != "" && !isMethodWrapper {
		return false
	}
	// go/ssa intentionally leaves Pkg nil for shared method wrappers. The
	// wrapped declaration still identifies their package and source position.
	var path string
	if fn.Pkg != nil && fn.Pkg.Pkg != nil {
		path = fn.Pkg.Pkg.Path()
	} else if isMethodWrapper {
		object, ok := fn.Object().(*types.Func)
		if !ok || object.Pkg() == nil {
			return false
		}
		path = object.Pkg().Path()
	} else {
		return false
	}
	if path == "runtime" || path == "internal/task" || (path == "main" && fn.Name() == "main") {
		return false
	}
	return c.isRegionsStrictFunction(fn) && !c.getFunctionInfo(fn).exported
}

// regionBlockCanUseRegion recognizes a deliberately narrow form of lexical
// lifetime: every allocation in the basic block is consumed in that same block
// without being stored, passed to a call, or merged through a phi. In a loop
// body, that block executes once per iteration, which gives us an iteration
// region without needing source-level loop reconstruction.
func (b *builder) regionBlockCanUseRegion(block *ssa.BasicBlock) bool {
	if !b.Regions || !b.regionsStrictPackage() {
		return false
	}
	hasAllocation := false
	for _, instr := range block.Instrs {
		value, ok := instr.(ssa.Value)
		if !ok || !b.regionInstructionAllocates(value) {
			continue
		}
		hasAllocation = true
		if !regionValueStaysInBlock(value, block) {
			return false
		}
	}
	return hasAllocation
}

func (b *builder) regionInstructionAllocates(value ssa.Value) bool {
	switch value := value.(type) {
	case *ssa.Alloc:
		if !value.Heap {
			return false
		}
		return b.targetData.TypeAllocSize(b.getLLVMType(value.Type().Underlying().(*types.Pointer).Elem())) != 0
	case *ssa.MakeChan, *ssa.MakeMap:
		return true
	case *ssa.MakeClosure:
		return b.regionClosureNeedsStorage(value)
	case *ssa.MakeInterface:
		return b.regionInterfaceNeedsStorage(value.X.Type())
	case *ssa.MakeSlice:
		return regionSliceNeedsStorage(value)
	case *ssa.BinOp:
		return value.Op.String() == "+" && typeIsString(value.X.Type())
	case *ssa.Convert:
		return regionConvertNeedsStorage(value)
	case *ssa.Call:
		builtin, ok := value.Call.Value.(*ssa.Builtin)
		return ok && builtin.Name() == "append"
	}
	return false
}

func regionBlockTerminator(instr ssa.Instruction) bool {
	switch instr.(type) {
	case *ssa.If, *ssa.Jump, *ssa.Return, *ssa.Panic:
		return true
	}
	return false
}

func regionValueStaysInBlock(root ssa.Value, block *ssa.BasicBlock) bool {
	seen := map[ssa.Value]bool{}
	work := []ssa.Value{root}
	for len(work) != 0 {
		value := work[len(work)-1]
		work = work[:len(work)-1]
		if seen[value] {
			continue
		}
		seen[value] = true
		refs := value.Referrers()
		if refs == nil {
			return false
		}
		for _, ref := range *refs {
			if ref.Block() != block {
				return false
			}
			switch ref := ref.(type) {
			case *ssa.DebugRef:
				continue
			case *ssa.MapUpdate:
				// A value stored in a caller-owned map escapes this lexical block.
				// Map updates to a map created in the same block remain local.
				if ref.Value == value && regionValueFromParameter(ref.Map, nil) {
					return false
				}
				continue
			case *ssa.Lookup:
				work = append(work, ref)
			case *ssa.IndexAddr, *ssa.FieldAddr, *ssa.Slice, *ssa.ChangeType, *ssa.ChangeInterface, *ssa.Convert:
				derived, ok := ref.(ssa.Value)
				if !ok {
					return false
				}
				work = append(work, derived)
			case *ssa.Store:
				// Storing through an address derived from this value initializes
				// memory inside the block-owned object. Storing the value itself
				// would create an alias whose lifetime is harder to prove.
				if ref.Val == value {
					return false
				}
			case *ssa.Call:
				builtin, ok := ref.Call.Value.(*ssa.Builtin)
				if !ok || (builtin.Name() != "len" && builtin.Name() != "cap" && builtin.Name() != "clear" && builtin.Name() != "delete") {
					return false
				}
			default:
				return false
			}
		}
	}
	return true
}

// regionOwner returns the most specific owner available at this point. A
// local function region wins; a hidden incoming owner propagates a caller's
// lifetime through arbitrary synchronous calls. Nil asks the runtime for the
// ambient owner and is deliberately not a root-region fallback.
func (b *builder) regionOwner() llvm.Value {
	if !b.blockRegionAlloca.IsNil() {
		return b.blockRegionAlloca
	}
	if !b.regionAlloca.IsNil() {
		return b.regionAlloca
	}
	if !b.regionOwnerParam.IsNil() {
		return b.regionOwnerParam
	}
	return llvm.ConstNull(b.dataPtrType)
}

// regionTargetOwner returns the owner selected by a slice/string target. A
// direct regions.Do literal records the captured storage it writes, allowing a
// later append or concat in the enclosing function to keep using that explicit
// Region instead of silently falling back to the enclosing automatic region.
func (b *builder) regionTargetOwner(value ssa.Value, pos token.Pos) (llvm.Value, error) {
	if owner, ok := b.regionManualOwnerForValue(value); ok {
		return owner, nil
	}
	return b.regionOwner(), nil
}

// regionTemporaryConvertOwner gives string([]byte(s)) and string([]rune(s)) a
// short-lived source-buffer owner when the final string escapes. The outer
// conversion copies the bytes/runes before regionReleaseTemporaryConvert runs,
// so only the intermediate buffer is released here; the string keeps its
// ordinary caller or explicit Region owner.
func (b *builder) regionTemporaryConvertOwner(convert *ssa.Convert) llvm.Value {
	if !b.Regions || !b.blockRegionAlloca.IsNil() || !regionConvertHasImmediateStringConsumer(convert) {
		return llvm.Value{}
	}
	regionType := b.getLLVMRuntimeType("region")
	owner := llvmutil.CreateEntryBlockAlloca(b.Builder, regionType, "region.convert")
	b.createRuntimeCall("regionEnter", []llvm.Value{owner}, "")
	b.regionConvertOwners[convert] = owner
	return owner
}

func (b *builder) regionReleaseTemporaryConvert(value ssa.Value) {
	convert, ok := value.(*ssa.Convert)
	if !ok {
		return
	}
	owner, ok := b.regionConvertOwners[convert]
	if !ok {
		return
	}
	b.createRuntimeCall("regionExit", []llvm.Value{owner}, "")
	delete(b.regionConvertOwners, convert)
}

// regionConvertHasImmediateStringConsumer recognizes precisely the source
// side of string([]byte(s)) and string([]rune(s)). Any other use could retain
// the slice, so it must continue to use its normal owner.
func regionConvertHasImmediateStringConsumer(convert *ssa.Convert) bool {
	from, ok := convert.X.Type().Underlying().(*types.Basic)
	if !ok || from.Kind() != types.String {
		return false
	}
	to, ok := convert.Type().Underlying().(*types.Slice)
	if !ok {
		return false
	}
	elem, ok := to.Elem().Underlying().(*types.Basic)
	if !ok || (elem.Kind() != types.Byte && elem.Kind() != types.Rune) {
		return false
	}
	refs := convert.Referrers()
	if refs == nil {
		return false
	}
	var consumer *ssa.Convert
	for _, ref := range *refs {
		if _, ok := ref.(*ssa.DebugRef); ok {
			continue
		}
		next, ok := ref.(*ssa.Convert)
		if !ok || next.X != convert || !typeIsString(next.Type()) {
			return false
		}
		if consumer != nil {
			return false
		}
		consumer = next
	}
	return consumer != nil
}

func (b *builder) regionManualOwnerForValue(value ssa.Value) (llvm.Value, bool) {
	if owner, ok := b.regionManualValueOwners[value]; ok {
		return owner, true
	}
	switch value := value.(type) {
	case *ssa.UnOp:
		if value.Op == token.MUL {
			return b.regionManualOwnerForValue(value.X)
		}
	case *ssa.FieldAddr:
		return b.regionManualOwnerForValue(value.X)
	case *ssa.Field:
		return b.regionManualOwnerForValue(value.X)
	case *ssa.IndexAddr:
		return b.regionManualOwnerForValue(value.X)
	case *ssa.Index:
		return b.regionManualOwnerForValue(value.X)
	case *ssa.ChangeType:
		return b.regionManualOwnerForValue(value.X)
	case *ssa.Convert:
		return b.regionManualOwnerForValue(value.X)
	case *ssa.Phi:
		var owner llvm.Value
		for _, edge := range value.Edges {
			edgeOwner, ok := b.regionManualOwnerForValue(edge)
			if !ok {
				return llvm.Value{}, false
			}
			if !owner.IsNil() && owner != edgeOwner {
				return llvm.Value{}, false
			}
			owner = edgeOwner
		}
		return owner, !owner.IsNil()
	}
	return llvm.Value{}, false
}

// regionExplicitOwnerForArgs extracts the one explicit owner that an internal
// call can propagate through its single hidden owner parameter. Calls carrying
// several distinct manual owners need a richer per-argument ABI, so reject
// them rather than silently selecting an unrelated region.
func (b *builder) regionExplicitOwnerForArgs(args []ssa.Value, pos token.Pos) (llvm.Value, bool, error) {
	var owner llvm.Value
	for _, arg := range args {
		if !isSliceOrString(arg.Type()) {
			continue
		}
		argOwner, ok := b.regionManualOwnerForValue(arg)
		if !ok {
			continue
		}
		if !owner.IsNil() && owner != argOwner {
			return llvm.Value{}, false, b.makeError(pos, "gc=regions cannot determine a target Region for a call with values from different explicit Regions")
		}
		owner = argOwner
	}
	return owner, !owner.IsNil(), nil
}

func isSliceOrString(t types.Type) bool {
	return isSliceType(t) || typeIsString(t)
}

func (b *builder) createManagedAlloc(size, layout llvm.Value, name string) llvm.Value {
	if !b.Regions {
		return b.createRuntimeCall("alloc", []llvm.Value{size, layout}, name)
	}
	return b.createRuntimeCall("regionAlloc", []llvm.Value{b.regionOwner(), size, layout}, name)
}

// regionClosureNeedsStorage mirrors emitPointerPack for the common case where
// the closure environment is a small scalar aggregate. Such an environment is
// represented directly in the func context pointer and does not need a region.
func (b *builder) regionClosureNeedsStorage(closure *ssa.MakeClosure) bool {
	valueTypes := make([]llvm.Type, len(closure.Bindings))
	for i, binding := range closure.Bindings {
		valueTypes[i] = b.getLLVMType(binding.Type())
	}
	packedType := b.ctx.StructType(valueTypes, false)
	size := b.targetData.TypeAllocSize(packedType)
	return size > b.targetData.TypeAllocSize(b.dataPtrType)
}

// regionInterfaceNeedsStorage mirrors the non-constant allocation path in
// emitPointerPack. Pointer-sized scalar interface values are embedded directly
// in their interface payload.
func (b *builder) regionInterfaceNeedsStorage(typ types.Type) bool {
	llvmType := b.getLLVMType(typ)
	if llvmType.TypeKind() == llvm.PointerTypeKind {
		return false
	}
	return b.targetData.TypeAllocSize(llvmType) > b.targetData.TypeAllocSize(b.dataPtrType)
}

func regionZeroSlice(slice *ssa.MakeSlice) bool {
	for _, value := range []ssa.Value{slice.Len, slice.Cap} {
		valueConst, ok := value.(*ssa.Const)
		if !ok || constant.Sign(valueConst.Value) != 0 {
			return false
		}
	}
	return true
}

// regionZeroSizedPointer provides zero-sized heap values with a stable non-nil
// address without involving the allocator. It is never dereferenced.
func (b *builder) regionZeroSizedPointer() llvm.Value {
	const name = "tinygo.regions.zeroSizedSlice"
	global := b.mod.NamedGlobal(name)
	if global.IsNil() {
		arrayType := llvm.ArrayType(b.ctx.Int8Type(), 1)
		global = llvm.AddGlobal(b.mod, arrayType, name)
		global.SetInitializer(llvm.ConstNull(arrayType))
		global.SetLinkage(llvm.InternalLinkage)
		global.SetGlobalConstant(true)
		global.SetUnnamedAddr(true)
		global.SetAlignment(1)
	}
	return global
}

// functionBorrowsCallerRegion is a compact interprocedural escape summary. A
// returned reference or a reference stored through a parameter needs the
// caller's owner. Merely accepting a map/slice/pointer does not: that common
// case can now use a short-lived function region for its temporaries.
func (b *builder) functionBorrowsCallerRegion() bool {
	if b.info.exported {
		return false
	}
	if results := b.fn.Signature.Results(); results != nil {
		for i := 0; i < results.Len(); i++ {
			if typeContainsReference(results.At(i).Type()) {
				return true
			}
		}
	}
	// Standard-library packages have not all been annotated with region
	// summaries yet. Keep their old conservative parameter contract while the
	// user package analysis gains precision.
	if !b.regionsStrictPackage() {
		for i := 0; i < b.fn.Signature.Params().Len(); i++ {
			if typeContainsReference(b.fn.Signature.Params().At(i).Type()) {
				return true
			}
		}
	}
	hasDefer := false
	for _, block := range b.fn.Blocks {
		for _, instruction := range block.Instrs {
			switch instruction := instruction.(type) {
			case *ssa.Defer:
				hasDefer = true
			case *ssa.Store:
				if typeContainsReference(instruction.Val.Type()) && regionValueFromParameter(instruction.Addr, nil) {
					return true
				}
			case *ssa.MapUpdate:
				if typeContainsReference(instruction.Value.Type()) && regionValueFromParameter(instruction.Map, nil) {
					return true
				}
			case *ssa.Send:
				if typeContainsReference(instruction.X.Type()) && regionValueFromParameter(instruction.Chan, nil) {
					return true
				}
			case *ssa.Select:
				for _, state := range instruction.States {
					if state.Dir == types.SendOnly && typeContainsReference(state.Send.Type()) && regionValueFromParameter(state.Chan, nil) {
						return true
					}
				}
			}
		}
	}
	// Deferred calls run immediately before this function's local region would
	// be released. Their bodies and arguments can write an allocation through
	// a reference parameter, including through a closure free variable. Keep
	// the enclosing function on the caller owner in that case.
	if hasDefer {
		params := b.fn.Signature.Params()
		for i := 0; i < params.Len(); i++ {
			if typeContainsReference(params.At(i).Type()) {
				return true
			}
		}
		for _, freeVar := range b.fn.FreeVars {
			if typeContainsReference(freeVar.Type()) {
				return true
			}
		}
	}
	return false
}

// functionNeedsRegion recognizes allocation paths lowered directly in this
// function. Calls into another function carry their own summary/owner, so a
// pure forwarding or arithmetic helper need not push a redundant region.
func (b *builder) functionNeedsRegion() bool {
	for _, block := range b.fn.Blocks {
		if b.regionBlockCanUseRegion(block) {
			continue
		}
		for _, instruction := range block.Instrs {
			switch instruction := instruction.(type) {
			case *ssa.Alloc:
				if b.targetData.TypeAllocSize(b.getLLVMType(instruction.Type().Underlying().(*types.Pointer).Elem())) != 0 {
					return true
				}
			case *ssa.MakeChan, *ssa.MakeMap, *ssa.Defer:
				return true
			case *ssa.MakeClosure:
				if b.regionClosureNeedsStorage(instruction) {
					return true
				}
			case *ssa.MakeInterface:
				if b.regionInterfaceNeedsStorage(instruction.X.Type()) {
					return true
				}
			case *ssa.MakeSlice:
				if regionSliceNeedsStorage(instruction) {
					return true
				}
			case *ssa.BinOp:
				if instruction.Op.String() == "+" && typeIsString(instruction.X.Type()) {
					return true
				}
			case *ssa.Convert:
				if regionConvertNeedsStorage(instruction) {
					return true
				}
			case *ssa.Call:
				if builtin, ok := instruction.Call.Value.(*ssa.Builtin); ok && builtin.Name() == "append" {
					return true
				}
			}
		}
	}
	return false
}

func regionSliceNeedsStorage(slice *ssa.MakeSlice) bool {
	capacity, ok := slice.Cap.(*ssa.Const)
	return !ok || constant.Sign(capacity.Value) != 0
}

// regionConvertNeedsStorage mirrors the conversion paths that call an
// allocating string*Regions runtime helper. Keeping it shared by block and
// function analysis prevents temporary conversion buffers from falling back
// to the caller owner.
func regionConvertNeedsStorage(convert *ssa.Convert) bool {
	from := convert.X.Type().Underlying()
	to := convert.Type().Underlying()
	if toBasic, ok := to.(*types.Basic); ok && toBasic.Kind() == types.String {
		switch from := from.(type) {
		case *types.Slice:
			elem, ok := from.Elem().Underlying().(*types.Basic)
			return ok && (elem.Kind() == types.Byte || elem.Kind() == types.Rune)
		case *types.Basic:
			return from.Info()&types.IsInteger != 0
		}
	}
	if toSlice, ok := to.(*types.Slice); ok {
		fromBasic, ok := from.(*types.Basic)
		if !ok || fromBasic.Kind() != types.String {
			return false
		}
		elem, ok := toSlice.Elem().Underlying().(*types.Basic)
		return ok && (elem.Kind() == types.Byte || elem.Kind() == types.Rune)
	}
	return false
}

func isDirectPointerType(t types.Type) bool {
	_, ok := t.Underlying().(*types.Pointer)
	return ok
}

func typeIsString(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.String
}

func isSliceType(t types.Type) bool {
	_, ok := t.Underlying().(*types.Slice)
	return ok
}

// regionValueFromParameter follows SSA operations that preserve a caller-owned
// reference. Unknown calls deliberately stop the walk, but projections and a
// direct load through a caller-derived address remain caller-owned.
func regionValueFromParameter(v ssa.Value, seen map[ssa.Value]bool) bool {
	if seen == nil {
		seen = make(map[ssa.Value]bool)
	}
	if seen[v] {
		return false
	}
	seen[v] = true
	switch v := v.(type) {
	case *ssa.Parameter:
		return true
	case *ssa.FreeVar:
		// A closure free variable is borrowed from its enclosing function. In a
		// deferred closure, the enclosing function may itself borrow the caller
		// owner, so this must not be treated as a closure-local destination.
		return true
	case *ssa.FieldAddr:
		return regionValueFromParameter(v.X, seen)
	case *ssa.Field:
		return regionValueFromParameter(v.X, seen)
	case *ssa.IndexAddr:
		return regionValueFromParameter(v.X, seen)
	case *ssa.Index:
		return regionValueFromParameter(v.X, seen)
	case *ssa.UnOp:
		return v.Op == token.MUL && regionValueFromParameter(v.X, seen)
	case *ssa.ChangeType:
		return regionValueFromParameter(v.X, seen)
	case *ssa.ChangeInterface:
		return regionValueFromParameter(v.X, seen)
	case *ssa.MakeInterface:
		return regionValueFromParameter(v.X, seen)
	case *ssa.Lookup:
		return regionValueFromParameter(v.X, seen)
	case *ssa.Phi:
		for _, edge := range v.Edges {
			if !regionValueFromParameter(edge, seen) {
				return false
			}
		}
		return len(v.Edges) != 0
	}
	return false
}

func typeContainsReference(t types.Type) bool {
	switch t := t.Underlying().(type) {
	case *types.Basic:
		// Strings and unsafe.Pointer carry data pointers even though they are
		// basic Go types. They need the same owner propagation as slices when
		// returned, stored, or sent to another task.
		return t.Kind() == types.String || t.Kind() == types.UnsafePointer
	case *types.Pointer, *types.Map, *types.Slice, *types.Chan, *types.Interface, *types.Signature:
		return true
	case *types.Array:
		return typeContainsReference(t.Elem())
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if typeContainsReference(t.Field(i).Type()) {
				return true
			}
		}
	}
	return false
}

func isManualRegionHandle(t types.Type) bool {
	ptr, ok := t.Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == "regions" && named.Obj().Name() == "Region"
}

func isUintptrType(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Uintptr
}

// regionStoreIsGlobal recognizes stores through a package global, including a
// field or array element reached from it. Stack and heap addresses are allowed
// because their owner is covered by the caller/function summary.
func regionStoreIsGlobal(addr ssa.Value) bool {
	switch addr := addr.(type) {
	case *ssa.Global:
		return true
	case *ssa.FieldAddr:
		return regionStoreIsGlobal(addr.X)
	case *ssa.IndexAddr:
		return regionStoreIsGlobal(addr.X)
	}
	return false
}

// createRegionsDo lowers regions.Do directly instead of calling through a Go
// callback. The direct closure runs while the explicit owner is active, so all
// normal allocations inside it use that owner. A panic unwinds it through the
// surrounding compiler-managed recover frame.
func (b *builder) createRegionsDo(instr *ssa.CallCommon) (llvm.Value, error) {
	if len(instr.Args) != 2 {
		return llvm.Value{}, b.makeError(instr.Pos(), "regions.Do expects a region and a direct function literal")
	}
	owner := b.getValue(instr.Args[0], getPos(instr.Args[0]))
	var fn *ssa.Function
	var closure *ssa.MakeClosure
	context := llvm.Undef(b.dataPtrType)
	switch value := instr.Args[1].(type) {
	case *ssa.MakeClosure:
		closure = value
		fn, _ = closure.Fn.(*ssa.Function)
		if fn == nil {
			return llvm.Value{}, b.makeError(instr.Pos(), "regions.Do requires a direct function literal")
		}
		context = b.extractFuncContext(b.getValue(closure, getPos(closure)))
	case *ssa.Function:
		// SSA represents a function literal without free variables as a plain
		// function value. It still has a statically known callee and needs no
		// closure context.
		fn = value
	default:
		return llvm.Value{}, b.makeError(instr.Pos(), "regions.Do requires a direct function literal")
	}
	if fn.Signature.Params().Len() != 0 || fn.Signature.Results().Len() != 0 {
		return llvm.Value{}, b.makeError(instr.Pos(), "regions.Do callback must have signature func()")
	}

	b.regionDoClosures[fn] = true
	if closure != nil {
		for i, freeVar := range fn.FreeVars {
			if i >= len(closure.Bindings) || !regionDoWritesFreeVar(fn, freeVar) {
				continue
			}
			bindingType, ok := closure.Bindings[i].Type().Underlying().(*types.Pointer)
			if ok && typeContainsReference(bindingType.Elem()) {
				b.regionManualValueOwners[closure.Bindings[i]] = owner
			}
		}
	}
	fnType, callee := b.getFunction(fn)
	b.createRuntimeCall("regionManualEnter", []llvm.Value{owner}, "")
	b.createInvoke(fnType, callee, []llvm.Value{context}, "")
	return b.createRuntimeCall("regionManualExit", []llvm.Value{owner}, ""), nil
}

func regionDoWritesFreeVar(fn *ssa.Function, freeVar *ssa.FreeVar) bool {
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			store, ok := instr.(*ssa.Store)
			if ok && regionValueDerivedFrom(store.Addr, freeVar, nil) {
				return true
			}
		}
	}
	return false
}

// regionValueDerivedFrom reports whether v is an address or projection
// derived from root. regions.Do uses it to record fields and indexed elements
// initialized while a manual owner is active.
func regionValueDerivedFrom(v, root ssa.Value, seen map[ssa.Value]bool) bool {
	if v == root {
		return true
	}
	if seen == nil {
		seen = make(map[ssa.Value]bool)
	}
	if seen[v] {
		return false
	}
	seen[v] = true
	switch v := v.(type) {
	case *ssa.FieldAddr:
		return regionValueDerivedFrom(v.X, root, seen)
	case *ssa.IndexAddr:
		return regionValueDerivedFrom(v.X, root, seen)
	case *ssa.ChangeType:
		return regionValueDerivedFrom(v.X, root, seen)
	case *ssa.UnOp:
		return v.Op == token.MUL && regionValueDerivedFrom(v.X, root, seen)
	}
	return false
}
