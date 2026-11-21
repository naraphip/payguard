# BIN master data attribution

The packaged `bins.csv.gz` adapts `bins.csv` from [Techbuddie Solutions / binlist.io](https://github.com/Techbuddie-Solutions/binlist-data/tree/fcc907c81cf02b12e396b0c12a8c16472b47619a). Techbuddie combines data from [Ian Nuttall](https://github.com/iannuttall/binlist-data) and [Venelin Kochev](https://github.com/venelinkochev/bin-list-data). Both source datasets and Techbuddie's merged release are available under [Creative Commons Attribution 4.0 International](LICENSE.bin-list-data.txt).

PayGuard selects the prefix, scheme, funding, issuer, country code, row source and last-seen date. It omits issuer contact details and out-of-scope five-digit prefixes, normalizes selected fields and compresses the result. The pinned upstream CSV has SHA-256 `50e9d57ae6c7b04c17a18c74aa3c9b0b07962ec4ba2fbe8d1ff3b6db00bd7615`.
