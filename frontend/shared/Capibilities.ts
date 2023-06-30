export enum Capibilities {
    '?', //unknown
    'T', //no DME, Transponder without mode A+C
    'X', //no DME, No Transponder
    'U', //no DME, Transponder with mode A+C
    'D', //DME, No Transponder
    'B', //DME, Transponder without mode A+C
    'A', //DME, Transponder with mode A+C
    'M', //TACAN only, No Transponder
    'N', //TACAN only, Transponder without mode A+C
    'P', //TACAN only, Transponder with mode A+C
    'Y', //simple RNAV, No Transponder
    'C', //simple RNAV, Transponder without mode A+C
    'I', //simple RNAV, Transponder with mode A+C
    'E', //advanced RNAV with Dual FMS
    'F', //advanced RNAV with Single FMS
    'G', //advanced RNAV with GPS or GNSS
    'R', //advanced RNAV with RNP capability
    'W', //advanced RNAV with RVSM capability
    'Q', //advanced RNAV with RNP and RVSM
}