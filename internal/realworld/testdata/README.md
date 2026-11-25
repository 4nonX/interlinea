# Files written by Trados Studio, TagEditor, MultiTerm, memoQ, Phrase and Wordfast

The tests in `internal/realworld` read these files, write a translation into
every segment that can take one, and read the result again; memories are
read and imported. The files were
written by the tools themselves, not by Interlinea, and come from public
repositories whose licences allow them to be copied. Each file is unchanged;
only its name is new.

| File | Written by | Source | Licence |
|---|---|---|---|
| `trados/disfr-change-tracking.sdlxliff` | Trados Studio, with tracked changes | [AlissaSabre/disfr](https://github.com/AlissaSabre/disfr) `UnitTestDisfrDoc/Samples/ChangeTracking_Trados_1.sdlxliff` | BSD, `licenses/disfr.txt` |
| `trados/disfr-spelling-checker.doc.sdlxliff` | Trados Studio, from Word | disfr `Configuring_Spelling_Checker.doc.sdlxliff` | BSD |
| `trados/disfr-language-support.doc.sdlxliff` | Trados Studio, from Word | disfr `Language_Support.doc.sdlxliff` | BSD |
| `trados/disfr-new-features.ppt.sdlxliff` | Trados Studio, from PowerPoint | disfr `New_features.ppt.sdlxliff` | BSD |
| `trados/rws-quotes.docx.sdlxliff` | Trados Studio, locked and signed-off segments | [RWS/Sdl-Community](https://github.com/RWS/Sdl-Community) `XLIFF Manager for Trados Studio/XLIFF.Manager.UnitTests/TestFiles/Sdlxliff/QuotesSample.docx.sdlxliff` | Apache 2.0, `licenses/rws-sdl-community.txt` |
| `trados/rws-tts.docx.sdlxliff` | Trados Studio, untranslated | RWS/Sdl-Community `SDLTM Import Plus/SDLTMImportUnitTests/TestingFiles/TTSExample.docx.sdlxliff` | Apache 2.0 |
| `trados/rws-quickplace.docx.sdlxliff` | Trados Studio | RWS/Sdl-Community `SDLTM Import Plus/SDLTMImportUnitTests/TestingFiles/#1 - quickplace.docx.sdlxliff` | Apache 2.0 |
| `trados/rws-photo-printer.doc.sdlxliff` | Trados Studio, one memory match | [RWS/trados-studio-api-samples](https://github.com/RWS/trados-studio-api-samples) `LanguagePlatform/TMAutomation/SamplePhotoPrinter.doc.sdlxliff` | Apache 2.0, `licenses/rws-trados-studio-api-samples.txt` |
| `trados/rws-perfect-match.docx.sdlxliff` | Trados Studio | [RWS/Sdl-studio-powershell-toolkit](https://github.com/RWS/Sdl-studio-powershell-toolkit) `Samples/ForPerfectMatch/de-DE/Translatable/sample1_word.docx.sdlxliff` | Apache 2.0, `licenses/rws-sdl-studio-powershell-toolkit.txt` |
| `trados/rws-sample.sdltb` | MultiTerm | RWS/trados-studio-api-samples `ProjectAutomation/Sdl.SDK.ProjectAutomation.Samples.Examples/SampleFiles/Sample.sdltb` | Apache 2.0 |
| `trados/rws-printer.sdltb` | MultiTerm | RWS/trados-studio-api-samples `.../SampleFiles/Termbase/Software.sdltb` | Apache 2.0 |
| `trados/sdltm-importer-fr-en.sdltm` | Trados Studio, a memory with tags | [diasks2/sdltm_importer](https://github.com/diasks2/sdltm_importer) `spec/sample_test_files/sample.sdltm` | MIT, `licenses/sdltm-importer.txt` |
| `trados/sdltm-importer-es-de.sdltm` | Trados Studio, a memory | sdltm_importer `spec/sample_test_files/sample_2.sdltm` | MIT |
| `trados/disfr-tea-party.sdltm` | Trados Studio, a memory | disfr `UnitTestDisfrDoc/Samples/tea-party.sdltm` | BSD |
| `trados/rws-general-en-de.sdltm` | Trados Studio, a memory with a list field | RWS/trados-studio-api-samples `.../SampleFiles/General En-De.sdltm` | Apache 2.0 |
| `wordfast/txt-tm-importer-pt-en.txt` | Wordfast, a memory in UTF-16 | [diasks2/txt_tm_importer](https://github.com/diasks2/txt_tm_importer) `spec/sample_files/wordfast_1.txt` | MIT, `licenses/txt-tm-importer.txt` |
| `wordfast/txt-tm-importer-en-es.txt` | Wordfast, a memory of a later version | txt_tm_importer `spec/sample_files/wordfast_2.txt` | MIT |
| `phrase/xlfql-ja-en.mxliff` | Phrase, from Word, with codes | [capybara-translation/xlfql](https://github.com/capybara-translation/xlfql) `data/sample.mxliff` | MIT, `licenses/xlfql.txt` |
| `wordfast/okapi-test01.docx.txml` | Wordfast Pro, from Word | the Okapi Framework's test files, as kept in [patois-labs/bilingua](https://github.com/patois-labs/bilingua) `bilingua-wordfast/tests/data/Test01.docx.txml` | Apache 2.0, `licenses/okapi-via-bilingua-apache-2.0.txt` |
| `wordfast/okapi-test02.html.txml` | Wordfast Pro, from HTML | Okapi, bilingua `bilingua-wordfast/tests/data/Test02.html.txml` | Apache 2.0 |
| `wordfast/okapi-test03.mif.txml` | Wordfast Pro, from FrameMaker | Okapi, bilingua `bilingua-wordfast/tests/data/Test03.mif.txml` | Apache 2.0 |
| `tageditor/okapi-test01.html.ttx` | TagEditor 8.2, from HTML | Okapi, bilingua `bilingua-ttx/tests/data/okapi_test01.ttx` | Apache 2.0 |
| `tageditor/okapi-test02-segmented.html.ttx` | TagEditor 8.2, segmented | Okapi, bilingua `bilingua-ttx/tests/data/okapi_test02_allseg.ttx` | Apache 2.0 |
| `tageditor/okapi-test02-unsegmented.html.ttx` | TagEditor 8.2, the same document never segmented | Okapi, bilingua `bilingua-ttx/tests/data/okapi_test02_noseg.ttx` | Apache 2.0 |
| `tageditor/okapi-issue-164.doc.ttx` | TagEditor 8.3, from Word, never segmented | Okapi, bilingua `bilingua-ttx/tests/data/okapi_issue_164.ttx` | Apache 2.0 |
| `memoq/disfr-change-tracking.mqxliff` | memoQ, with tracked changes | disfr `ChangeTracking_memoQ_1.mqxliff` | BSD |
| `memoq/disfr-git.html.mqxlz` | memoQ, archive of an HTML document | disfr `git.html_jpn.mqxlz` | BSD |
| `memoq/xliffkit-ph.mqxliff` | memoQ | [ikuehirata/xliffkit](https://github.com/ikuehirata/xliffkit) `examples/01_fix_ph_to_bpt_ept/input.mqxliff` | MIT, `licenses/xliffkit.txt` |
| `memoq/xliffkit-split.mqxliff` | memoQ | xliffkit `examples/02_split/output.mqxliff` | MIT |
| `memoq/xliffkit-translated.mqxliff` | memoQ, translated | xliffkit `examples/03_merge/input.mqxliff` | MIT |

The repositories were read at these commits: disfr `7f47558`, xliffkit
`a1db2b7`, trados-studio-api-samples `4cf88a2`, Sdl-Community `70229c4`,
Sdl-studio-powershell-toolkit `4f45d9a`, sdltm_importer `8b2df47`,
txt_tm_importer `f6b8fd9`, xlfql `119f1d6`, bilingua `e5e0e5d`.

These files are older than Trados Studio 2024 and memoQ 11, and none is a
project package (.sdlppx, .sdlrpx) or a memoQ handoff package. Files from
current versions, and a return package opened in Trados and memoQ, are
still to be tested.
