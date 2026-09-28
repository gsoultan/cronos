// Code generated from H3 v4.1.0's src/h3lib/lib/baseCells.c; DO NOT EDIT.
//
// Copyright 2016-2021 Uber Technologies, Inc. Licensed under the Apache
// License, Version 2.0 — see LICENSE in this directory.

package h3

// faceBaseCells is, for each face and res-0 ijk+ coordinate (0 to 2 on each
// axis), the base cell there and the ccw 60° rotations into its coordinate
// system: faceIjkBaseCells, flattened as face*27 + i*9 + j*3 + k.
var faceBaseCells = [20 * 27]baseCellRotation{
	{16, 0}, {18, 0}, {24, 0}, {33, 0}, {30, 0}, {32, 3}, {49, 1}, {48, 3}, {50, 3},
	{8, 0}, {5, 5}, {10, 5}, {22, 0}, {16, 0}, {18, 0}, {41, 1}, {33, 0}, {30, 0},
	{4, 0}, {0, 5}, {2, 5}, {15, 1}, {8, 0}, {5, 5}, {31, 1}, {22, 0}, {16, 0},
	{2, 0}, {6, 0}, {14, 0}, {10, 0}, {11, 0}, {17, 3}, {24, 1}, {23, 3}, {25, 3},
	{0, 0}, {1, 5}, {9, 5}, {5, 0}, {2, 0}, {6, 0}, {18, 1}, {10, 0}, {11, 0},
	{4, 1}, {3, 5}, {7, 5}, {8, 1}, {0, 0}, {1, 5}, {16, 1}, {5, 0}, {2, 0},
	{7, 0}, {21, 0}, {38, 0}, {9, 0}, {19, 0}, {34, 3}, {14, 1}, {20, 3}, {36, 3},
	{3, 0}, {13, 5}, {29, 5}, {1, 0}, {7, 0}, {21, 0}, {6, 1}, {9, 0}, {19, 0},
	{4, 2}, {12, 5}, {26, 5}, {0, 1}, {3, 0}, {13, 5}, {2, 1}, {1, 0}, {7, 0},
	{26, 0}, {42, 0}, {58, 0}, {29, 0}, {43, 0}, {62, 3}, {38, 1}, {47, 3}, {64, 3},
	{12, 0}, {28, 5}, {44, 5}, {13, 0}, {26, 0}, {42, 0}, {21, 1}, {29, 0}, {43, 0},
	{4, 3}, {15, 5}, {31, 5}, {3, 1}, {12, 0}, {28, 5}, {7, 1}, {13, 0}, {26, 0},
	{31, 0}, {41, 0}, {49, 0}, {44, 0}, {53, 0}, {61, 3}, {58, 1}, {65, 3}, {75, 3},
	{15, 0}, {22, 5}, {33, 5}, {28, 0}, {31, 0}, {41, 0}, {42, 1}, {44, 0}, {53, 0},
	{4, 4}, {8, 5}, {16, 5}, {12, 1}, {15, 0}, {22, 5}, {26, 1}, {28, 0}, {31, 0},
	{50, 0}, {48, 0}, {49, 3}, {32, 0}, {30, 3}, {33, 3}, {24, 3}, {18, 3}, {16, 3},
	{70, 0}, {67, 0}, {66, 3}, {52, 3}, {50, 0}, {48, 0}, {37, 3}, {32, 0}, {30, 3},
	{83, 0}, {87, 3}, {85, 3}, {74, 3}, {70, 0}, {67, 0}, {57, 1}, {52, 3}, {50, 0},
	{25, 0}, {23, 0}, {24, 3}, {17, 0}, {11, 3}, {10, 3}, {14, 3}, {6, 3}, {2, 3},
	{45, 0}, {39, 0}, {37, 3}, {35, 3}, {25, 0}, {23, 0}, {27, 3}, {17, 0}, {11, 3},
	{63, 0}, {59, 3}, {57, 3}, {56, 3}, {45, 0}, {39, 0}, {46, 3}, {35, 3}, {25, 0},
	{36, 0}, {20, 0}, {14, 3}, {34, 0}, {19, 3}, {9, 3}, {38, 3}, {21, 3}, {7, 3},
	{55, 0}, {40, 0}, {27, 3}, {54, 3}, {36, 0}, {20, 0}, {51, 3}, {34, 0}, {19, 3},
	{72, 0}, {60, 3}, {46, 3}, {73, 3}, {55, 0}, {40, 0}, {71, 3}, {54, 3}, {36, 0},
	{64, 0}, {47, 0}, {38, 3}, {62, 0}, {43, 3}, {29, 3}, {58, 3}, {42, 3}, {26, 3},
	{84, 0}, {69, 0}, {51, 3}, {82, 3}, {64, 0}, {47, 0}, {76, 3}, {62, 0}, {43, 3},
	{97, 0}, {89, 3}, {71, 3}, {98, 3}, {84, 0}, {69, 0}, {96, 3}, {82, 3}, {64, 0},
	{75, 0}, {65, 0}, {58, 3}, {61, 0}, {53, 3}, {44, 3}, {49, 3}, {41, 3}, {31, 3},
	{94, 0}, {86, 0}, {76, 3}, {81, 3}, {75, 0}, {65, 0}, {66, 3}, {61, 0}, {53, 3},
	{107, 0}, {104, 3}, {96, 3}, {101, 3}, {94, 0}, {86, 0}, {85, 3}, {81, 3}, {75, 0},
	{57, 0}, {59, 0}, {63, 3}, {74, 0}, {78, 3}, {79, 3}, {83, 3}, {92, 3}, {95, 3},
	{37, 0}, {39, 3}, {45, 3}, {52, 0}, {57, 0}, {59, 0}, {70, 3}, {74, 0}, {78, 3},
	{24, 0}, {23, 3}, {25, 3}, {32, 3}, {37, 0}, {39, 3}, {50, 3}, {52, 0}, {57, 0},
	{46, 0}, {60, 0}, {72, 3}, {56, 0}, {68, 3}, {80, 3}, {63, 3}, {77, 3}, {90, 3},
	{27, 0}, {40, 3}, {55, 3}, {35, 0}, {46, 0}, {60, 0}, {45, 3}, {56, 0}, {68, 3},
	{14, 0}, {20, 3}, {36, 3}, {17, 3}, {27, 0}, {40, 3}, {25, 3}, {35, 0}, {46, 0},
	{71, 0}, {89, 0}, {97, 3}, {73, 0}, {91, 3}, {103, 3}, {72, 3}, {88, 3}, {105, 3},
	{51, 0}, {69, 3}, {84, 3}, {54, 0}, {71, 0}, {89, 0}, {55, 3}, {73, 0}, {91, 3},
	{38, 0}, {47, 3}, {64, 3}, {34, 3}, {51, 0}, {69, 3}, {36, 3}, {54, 0}, {71, 0},
	{96, 0}, {104, 0}, {107, 3}, {98, 0}, {110, 3}, {115, 3}, {97, 3}, {111, 3}, {119, 3},
	{76, 0}, {86, 3}, {94, 3}, {82, 0}, {96, 0}, {104, 0}, {84, 3}, {98, 0}, {110, 3},
	{58, 0}, {65, 3}, {75, 3}, {62, 3}, {76, 0}, {86, 3}, {64, 3}, {82, 0}, {96, 0},
	{85, 0}, {87, 0}, {83, 3}, {101, 0}, {102, 3}, {100, 3}, {107, 3}, {112, 3}, {114, 3},
	{66, 0}, {67, 3}, {70, 3}, {81, 0}, {85, 0}, {87, 0}, {94, 3}, {101, 0}, {102, 3},
	{49, 0}, {48, 3}, {50, 3}, {61, 3}, {66, 0}, {67, 3}, {75, 3}, {81, 0}, {85, 0},
	{95, 0}, {92, 0}, {83, 0}, {79, 0}, {78, 0}, {74, 3}, {63, 1}, {59, 3}, {57, 3},
	{109, 0}, {108, 0}, {100, 5}, {93, 1}, {95, 0}, {92, 0}, {77, 1}, {79, 0}, {78, 0},
	{117, 4}, {118, 5}, {114, 5}, {106, 1}, {109, 0}, {108, 0}, {90, 1}, {93, 1}, {95, 0},
	{90, 0}, {77, 0}, {63, 0}, {80, 0}, {68, 0}, {56, 3}, {72, 1}, {60, 3}, {46, 3},
	{106, 0}, {93, 0}, {79, 5}, {99, 1}, {90, 0}, {77, 0}, {88, 1}, {80, 0}, {68, 0},
	{117, 3}, {109, 5}, {95, 5}, {113, 1}, {106, 0}, {93, 0}, {105, 1}, {99, 1}, {90, 0},
	{105, 0}, {88, 0}, {72, 0}, {103, 0}, {91, 0}, {73, 3}, {97, 1}, {89, 3}, {71, 3},
	{113, 0}, {99, 0}, {80, 5}, {116, 1}, {105, 0}, {88, 0}, {111, 1}, {103, 0}, {91, 0},
	{117, 2}, {106, 5}, {90, 5}, {121, 1}, {113, 0}, {99, 0}, {119, 1}, {116, 1}, {105, 0},
	{119, 0}, {111, 0}, {97, 0}, {115, 0}, {110, 0}, {98, 3}, {107, 1}, {104, 3}, {96, 3},
	{121, 0}, {116, 0}, {103, 5}, {120, 1}, {119, 0}, {111, 0}, {112, 1}, {115, 0}, {110, 0},
	{117, 1}, {113, 5}, {105, 5}, {118, 1}, {121, 0}, {116, 0}, {114, 1}, {120, 1}, {119, 0},
	{114, 0}, {112, 0}, {107, 0}, {100, 0}, {102, 0}, {101, 3}, {83, 1}, {87, 3}, {85, 3},
	{118, 0}, {120, 0}, {115, 5}, {108, 1}, {114, 0}, {112, 0}, {92, 1}, {100, 0}, {102, 0},
	{117, 0}, {121, 5}, {119, 5}, {109, 1}, {118, 0}, {120, 0}, {95, 1}, {108, 1}, {114, 0},
}

// baseCells is each base cell's home face and ijk+ coordinates on it, whether
// it is a pentagon, and a pentagon's two clockwise offset faces.
var baseCells = [122]baseCell{
	{faceIJK{1, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},   // 0
	{faceIJK{2, coordIJK{1, 1, 0}}, false, [2]int{0, 0}},   // 1
	{faceIJK{1, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},   // 2
	{faceIJK{2, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},   // 3
	{faceIJK{0, coordIJK{2, 0, 0}}, true, [2]int{-1, -1}},  // 4
	{faceIJK{1, coordIJK{1, 1, 0}}, false, [2]int{0, 0}},   // 5
	{faceIJK{1, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},   // 6
	{faceIJK{2, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},   // 7
	{faceIJK{0, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},   // 8
	{faceIJK{2, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},   // 9
	{faceIJK{1, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},   // 10
	{faceIJK{1, coordIJK{0, 1, 1}}, false, [2]int{0, 0}},   // 11
	{faceIJK{3, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},   // 12
	{faceIJK{3, coordIJK{1, 1, 0}}, false, [2]int{0, 0}},   // 13
	{faceIJK{11, coordIJK{2, 0, 0}}, true, [2]int{2, 6}},   // 14
	{faceIJK{4, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},   // 15
	{faceIJK{0, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},   // 16
	{faceIJK{6, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},   // 17
	{faceIJK{0, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},   // 18
	{faceIJK{2, coordIJK{0, 1, 1}}, false, [2]int{0, 0}},   // 19
	{faceIJK{7, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},   // 20
	{faceIJK{2, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},   // 21
	{faceIJK{0, coordIJK{1, 1, 0}}, false, [2]int{0, 0}},   // 22
	{faceIJK{6, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},   // 23
	{faceIJK{10, coordIJK{2, 0, 0}}, true, [2]int{1, 5}},   // 24
	{faceIJK{6, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},   // 25
	{faceIJK{3, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},   // 26
	{faceIJK{11, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},  // 27
	{faceIJK{4, coordIJK{1, 1, 0}}, false, [2]int{0, 0}},   // 28
	{faceIJK{3, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},   // 29
	{faceIJK{0, coordIJK{0, 1, 1}}, false, [2]int{0, 0}},   // 30
	{faceIJK{4, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},   // 31
	{faceIJK{5, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},   // 32
	{faceIJK{0, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},   // 33
	{faceIJK{7, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},   // 34
	{faceIJK{11, coordIJK{1, 1, 0}}, false, [2]int{0, 0}},  // 35
	{faceIJK{7, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},   // 36
	{faceIJK{10, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},  // 37
	{faceIJK{12, coordIJK{2, 0, 0}}, true, [2]int{3, 7}},   // 38
	{faceIJK{6, coordIJK{1, 0, 1}}, false, [2]int{0, 0}},   // 39
	{faceIJK{7, coordIJK{1, 0, 1}}, false, [2]int{0, 0}},   // 40
	{faceIJK{4, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},   // 41
	{faceIJK{3, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},   // 42
	{faceIJK{3, coordIJK{0, 1, 1}}, false, [2]int{0, 0}},   // 43
	{faceIJK{4, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},   // 44
	{faceIJK{6, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},   // 45
	{faceIJK{11, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},  // 46
	{faceIJK{8, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},   // 47
	{faceIJK{5, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},   // 48
	{faceIJK{14, coordIJK{2, 0, 0}}, true, [2]int{0, 9}},   // 49
	{faceIJK{5, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},   // 50
	{faceIJK{12, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},  // 51
	{faceIJK{10, coordIJK{1, 1, 0}}, false, [2]int{0, 0}},  // 52
	{faceIJK{4, coordIJK{0, 1, 1}}, false, [2]int{0, 0}},   // 53
	{faceIJK{12, coordIJK{1, 1, 0}}, false, [2]int{0, 0}},  // 54
	{faceIJK{7, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},   // 55
	{faceIJK{11, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},  // 56
	{faceIJK{10, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},  // 57
	{faceIJK{13, coordIJK{2, 0, 0}}, true, [2]int{4, 8}},   // 58
	{faceIJK{10, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},  // 59
	{faceIJK{11, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},  // 60
	{faceIJK{9, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},   // 61
	{faceIJK{8, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},   // 62
	{faceIJK{6, coordIJK{2, 0, 0}}, true, [2]int{11, 15}},  // 63
	{faceIJK{8, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},   // 64
	{faceIJK{9, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},   // 65
	{faceIJK{14, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},  // 66
	{faceIJK{5, coordIJK{1, 0, 1}}, false, [2]int{0, 0}},   // 67
	{faceIJK{16, coordIJK{0, 1, 1}}, false, [2]int{0, 0}},  // 68
	{faceIJK{8, coordIJK{1, 0, 1}}, false, [2]int{0, 0}},   // 69
	{faceIJK{5, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},   // 70
	{faceIJK{12, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},  // 71
	{faceIJK{7, coordIJK{2, 0, 0}}, true, [2]int{12, 16}},  // 72
	{faceIJK{12, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},  // 73
	{faceIJK{10, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},  // 74
	{faceIJK{9, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},   // 75
	{faceIJK{13, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},  // 76
	{faceIJK{16, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},  // 77
	{faceIJK{15, coordIJK{0, 1, 1}}, false, [2]int{0, 0}},  // 78
	{faceIJK{15, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},  // 79
	{faceIJK{16, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},  // 80
	{faceIJK{14, coordIJK{1, 1, 0}}, false, [2]int{0, 0}},  // 81
	{faceIJK{13, coordIJK{1, 1, 0}}, false, [2]int{0, 0}},  // 82
	{faceIJK{5, coordIJK{2, 0, 0}}, true, [2]int{10, 19}},  // 83
	{faceIJK{8, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},   // 84
	{faceIJK{14, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},  // 85
	{faceIJK{9, coordIJK{1, 0, 1}}, false, [2]int{0, 0}},   // 86
	{faceIJK{14, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},  // 87
	{faceIJK{17, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},  // 88
	{faceIJK{12, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},  // 89
	{faceIJK{16, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},  // 90
	{faceIJK{17, coordIJK{0, 1, 1}}, false, [2]int{0, 0}},  // 91
	{faceIJK{15, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},  // 92
	{faceIJK{16, coordIJK{1, 0, 1}}, false, [2]int{0, 0}},  // 93
	{faceIJK{9, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},   // 94
	{faceIJK{15, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},  // 95
	{faceIJK{13, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},  // 96
	{faceIJK{8, coordIJK{2, 0, 0}}, true, [2]int{13, 17}},  // 97
	{faceIJK{13, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},  // 98
	{faceIJK{17, coordIJK{1, 0, 1}}, false, [2]int{0, 0}},  // 99
	{faceIJK{19, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},  // 100
	{faceIJK{14, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},  // 101
	{faceIJK{19, coordIJK{0, 1, 1}}, false, [2]int{0, 0}},  // 102
	{faceIJK{17, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},  // 103
	{faceIJK{13, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},  // 104
	{faceIJK{17, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},  // 105
	{faceIJK{16, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},  // 106
	{faceIJK{9, coordIJK{2, 0, 0}}, true, [2]int{14, 18}},  // 107
	{faceIJK{15, coordIJK{1, 0, 1}}, false, [2]int{0, 0}},  // 108
	{faceIJK{15, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},  // 109
	{faceIJK{18, coordIJK{0, 1, 1}}, false, [2]int{0, 0}},  // 110
	{faceIJK{18, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},  // 111
	{faceIJK{19, coordIJK{0, 0, 1}}, false, [2]int{0, 0}},  // 112
	{faceIJK{17, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},  // 113
	{faceIJK{19, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},  // 114
	{faceIJK{18, coordIJK{0, 1, 0}}, false, [2]int{0, 0}},  // 115
	{faceIJK{18, coordIJK{1, 0, 1}}, false, [2]int{0, 0}},  // 116
	{faceIJK{19, coordIJK{2, 0, 0}}, true, [2]int{-1, -1}}, // 117
	{faceIJK{19, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},  // 118
	{faceIJK{18, coordIJK{0, 0, 0}}, false, [2]int{0, 0}},  // 119
	{faceIJK{19, coordIJK{1, 0, 1}}, false, [2]int{0, 0}},  // 120
	{faceIJK{18, coordIJK{1, 0, 0}}, false, [2]int{0, 0}},  // 121
}
