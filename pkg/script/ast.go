package script

// Expr is a Lua expression node.
type Expr interface{ isExpr() }

// Stmt is a Lua statement node.
type Stmt interface{ isStmt() }

// NilLit is the literal nil.
type NilLit struct{}

// BoolLit is a boolean literal.
type BoolLit struct{ Value bool }

// IntLit is an integer literal.
type IntLit struct{ Value int64 }

// FloatLit is a float literal.
type FloatLit struct{ Value float64 }

// StrLit is a string literal.
type StrLit struct{ Value string }

// Vararg is the "..." expression.
type Vararg struct{}

// Name references a variable.
type Name struct{ Name string }

// Index is x[key] or x.name.
type Index struct {
	X   Expr
	Key Expr
}

// Call is fn(args) or obj:method(args).
type Call struct {
	Fn     Expr
	Method string
	Args   []Expr
}

// Unop is a unary operation.
type Unop struct {
	Op string
	X  Expr
}

// Binop is a binary operation.
type Binop struct {
	Op   string
	L, R Expr
}

// TableLit is a table constructor.
type TableLit struct{ Fields []TableField }

// TableField is one entry of a table constructor. A nil Key is an array item.
type TableField struct {
	Key   Expr
	Value Expr
}

// FuncLit is a function expression.
type FuncLit struct {
	Params []string
	Vararg bool
	Body   []Stmt
}

func (*NilLit) isExpr()   {}
func (*BoolLit) isExpr()  {}
func (*IntLit) isExpr()   {}
func (*FloatLit) isExpr() {}
func (*StrLit) isExpr()   {}
func (*Vararg) isExpr()   {}
func (*Name) isExpr()     {}
func (*Index) isExpr()    {}
func (*Call) isExpr()     {}
func (*Unop) isExpr()     {}
func (*Binop) isExpr()    {}
func (*TableLit) isExpr() {}
func (*FuncLit) isExpr()  {}

// LocalStmt declares locals: local a, b = 1, 2.
type LocalStmt struct {
	Names []string
	Exprs []Expr
}

// AssignStmt assigns to existing targets: a, b = 1, 2.
type AssignStmt struct {
	Targets []Expr
	Exprs   []Expr
}

// LocalFuncStmt is local function f() ... end.
type LocalFuncStmt struct {
	Name string
	Fn   *FuncLit
}

// FuncStmt is function target() ... end.
type FuncStmt struct {
	Target Expr
	Method string
	Fn     *FuncLit
}

// CallStmt is a call used as a statement.
type CallStmt struct{ Call Expr }

// DoStmt is a do ... end block.
type DoStmt struct{ Body []Stmt }

// ElseIf is one elseif branch.
type ElseIf struct {
	Cond Expr
	Body []Stmt
}

// IfStmt is an if/elseif/else chain.
type IfStmt struct {
	Cond    Expr
	Then    []Stmt
	ElseIfs []ElseIf
	Else    []Stmt
}

// WhileStmt is a while loop.
type WhileStmt struct {
	Cond Expr
	Body []Stmt
}

// RepeatStmt is a repeat/until loop.
type RepeatStmt struct {
	Body []Stmt
	Cond Expr
}

// NumForStmt is a numeric for loop.
type NumForStmt struct {
	Name               string
	Start, Limit, Step Expr
	Body               []Stmt
}

// GenForStmt is a generic for loop.
type GenForStmt struct {
	Names []string
	Exprs []Expr
	Body  []Stmt
}

// ReturnStmt returns from a function.
type ReturnStmt struct{ Exprs []Expr }

// BreakStmt breaks the innermost loop.
type BreakStmt struct{}

func (*LocalStmt) isStmt()     {}
func (*AssignStmt) isStmt()    {}
func (*LocalFuncStmt) isStmt() {}
func (*FuncStmt) isStmt()      {}
func (*CallStmt) isStmt()      {}
func (*DoStmt) isStmt()        {}
func (*IfStmt) isStmt()        {}
func (*WhileStmt) isStmt()     {}
func (*RepeatStmt) isStmt()    {}
func (*NumForStmt) isStmt()    {}
func (*GenForStmt) isStmt()    {}
func (*ReturnStmt) isStmt()    {}
func (*BreakStmt) isStmt()     {}
