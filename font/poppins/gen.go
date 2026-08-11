package poppins

//go:generate go run seedhammer.com/cmd/bitmapfont -package poppins -ppem 16 Poppins-Regular.ttf regular

//go:generate go run seedhammer.com/cmd/bitmapfont -package poppins -ppem 10 Poppins-Bold.ttf bold
//go:generate go run seedhammer.com/cmd/bitmapfont -package poppins -ppem 16 Poppins-Bold.ttf bold
//go:generate go run seedhammer.com/cmd/bitmapfont -package poppins -ppem 20 Poppins-Bold.ttf bold
//go:generate go run seedhammer.com/cmd/bitmapfont -package poppins -ppem 25 Poppins-Bold.ttf bold
// Size 45 is only for progress indicators. "%" was added for F-86: the KDF
// progress screen formats its reading as "%d%%" in this face, and without
// "%" in the alphabet the percent sign rendered as zero pixels for the
// whole ~31s derivation.
//go:generate go run seedhammer.com/cmd/bitmapfont -package poppins -ppem 45 -alphabet "0123456789:%" Poppins-Bold.ttf boldprogress
