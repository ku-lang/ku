// Code generated by "stringer -type=PrimitiveType"; DO NOT EDIT

package ast

import "fmt"

const _PrimitiveType_name = "PRIMITIVE_s8PRIMITIVE_s16PRIMITIVE_s32PRIMITIVE_s64PRIMITIVE_s128PRIMITIVE_u8PRIMITIVE_u16PRIMITIVE_u32PRIMITIVE_u64PRIMITIVE_u128PRIMITIVE_f32PRIMITIVE_f64PRIMITIVE_f128PRIMITIVE_intPRIMITIVE_uintPRIMITIVE_uintptrPRIMITIVE_boolPRIMITIVE_void"

var _PrimitiveType_index = [...]uint8{0, 12, 25, 38, 51, 65, 77, 90, 103, 116, 130, 143, 156, 170, 183, 197, 214, 228, 242}

func (i PrimitiveType) String() string {
	if i < 0 || i >= PrimitiveType(len(_PrimitiveType_index)-1) {
		return fmt.Sprintf("PrimitiveType(%d)", i)
	}
	return _PrimitiveType_name[_PrimitiveType_index[i]:_PrimitiveType_index[i+1]]
}
