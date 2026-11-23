// hunspell-check AFF DIC [-s]: the reference checker for spell/diff_test.go.
// Build: g++ -O2 -o hunspell-check hunspell-check.cxx -lhunspell-1.7
// Reads UTF-8 words, one per line; prints "1 word" or "0 word", with -s
// also a tab and the suggestions joined by ", ".
#include <hunspell/hunspell.hxx>
#include <iconv.h>
#include <iostream>
#include <string>
#include <vector>
static std::string conv(const std::string& in, const char* to, const char* from) {
  iconv_t cd = iconv_open(to, from);
  std::string out(in.size() * 4 + 16, '\0');
  char* ip = const_cast<char*>(in.data()); size_t il = in.size();
  char* op = &out[0]; size_t ol = out.size();
  if (iconv(cd, &ip, &il, &op, &ol) == (size_t)-1) { iconv_close(cd); return std::string(); }
  iconv_close(cd);
  out.resize(out.size() - ol);
  return out;
}
int main(int argc, char** argv) {
  Hunspell h(argv[1], argv[2]);
  bool sug = argc > 3;
  std::string enc = h.get_dict_encoding();
  bool utf8 = enc == "UTF-8";
  if (enc == "microsoft-cp1251") enc = "CP1251";
  std::string line;
  while (std::getline(std::cin, line)) {
    std::string w = utf8 ? line : conv(line, enc.c_str(), "UTF-8");
    if (w.empty()) { std::cout << "? " << line << "\n"; continue; }
    bool ok = h.spell(w);
    std::cout << (ok ? 1 : 0) << " " << line;
    if (sug && !ok) {
      std::vector<std::string> s = h.suggest(w);
      std::cout << "\t";
      for (size_t i = 0; i < s.size(); i++) std::cout << (i ? ", " : "") << (utf8 ? s[i] : conv(s[i], "UTF-8", enc.c_str()));
    }
    std::cout << "\n";
  }
}
