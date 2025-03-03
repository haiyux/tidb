// Copyright 2017 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aggregation

import (
	"github.com/pingcap/tidb/pkg/expression"
	"github.com/pingcap/tidb/pkg/parser/mysql"
	"github.com/pingcap/tidb/pkg/parser/terror"
	"github.com/pingcap/tidb/pkg/sessionctx/stmtctx"
	"github.com/pingcap/tidb/pkg/types"
	"github.com/pingcap/tidb/pkg/util/chunk"
	"github.com/pingcap/tidb/pkg/util/mathutil"
)

type avgFunction struct {
	aggFunction
}

func (af *avgFunction) updateAvg(ctx types.Context, evalCtx *AggEvaluateContext, row chunk.Row) error {
	a := af.Args[1]
	value, err := a.Eval(evalCtx.Ctx, row)
	if err != nil {
		return err
	}
	if value.IsNull() {
		return nil
	}
	evalCtx.Value, err = calculateSum(ctx, evalCtx.Value, value)
	if err != nil {
		return err
	}
	count, err := af.Args[0].Eval(evalCtx.Ctx, row)
	if err != nil {
		return err
	}
	evalCtx.Count += count.GetInt64()
	return nil
}

func (af *avgFunction) ResetContext(ctx expression.EvalContext, evalCtx *AggEvaluateContext) {
	if af.HasDistinct {
		evalCtx.DistinctChecker = createDistinctChecker(ctx)
	}
	evalCtx.Ctx = ctx
	evalCtx.Value.SetNull()
	evalCtx.Count = 0
}

// Update implements Aggregation interface.
func (af *avgFunction) Update(evalCtx *AggEvaluateContext, sc *stmtctx.StatementContext, row chunk.Row) (err error) {
	switch af.Mode {
	case Partial1Mode, CompleteMode:
		err = af.updateSum(sc.TypeCtx(), evalCtx, row)
	case Partial2Mode, FinalMode:
		err = af.updateAvg(sc.TypeCtx(), evalCtx, row)
	case DedupMode:
		panic("DedupMode is not supported now.")
	}
	return err
}

// GetResult implements Aggregation interface.
func (af *avgFunction) GetResult(evalCtx *AggEvaluateContext) (d types.Datum) {
	switch evalCtx.Value.Kind() {
	case types.KindFloat64:
		sum := evalCtx.Value.GetFloat64()
		d.SetFloat64(sum / float64(evalCtx.Count))
		return
	case types.KindMysqlDecimal:
		x := evalCtx.Value.GetMysqlDecimal()
		y := types.NewDecFromInt(evalCtx.Count)
		to := new(types.MyDecimal)
		err := types.DecimalDiv(x, y, to, types.DivFracIncr)
		terror.Log(err)
		frac := af.RetTp.GetDecimal()
		if frac == -1 {
			frac = mysql.MaxDecimalScale
		}
		err = to.Round(to, mathutil.Min(frac, mysql.MaxDecimalScale), types.ModeHalfUp)
		terror.Log(err)
		d.SetMysqlDecimal(to)
	}
	return
}

// GetPartialResult implements Aggregation interface.
func (*avgFunction) GetPartialResult(evalCtx *AggEvaluateContext) []types.Datum {
	return []types.Datum{types.NewIntDatum(evalCtx.Count), evalCtx.Value}
}
