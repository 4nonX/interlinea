package langmatrix

// sample is a short text in a language, with what the checks need to know
// about it. The texts say the same in every language: "The contract begins
// today. The tenant pays the rent to the landlord every month, and the
// deposit is not due this month."
type sample struct {
	// lang is the primary language, or a full tag where the script or the
	// region matters (zh-TW, sr-Latn-RS, es-MX).
	lang string
	text string
	// sentences is how many sentences the text splits into; 0 means two.
	// Thai and Lao write no full stop, so theirs stays one.
	sentences int
	// term is the word for "contract" as a termbase would hold it, and
	// form a piece of running text with the word in another form.
	term, form string
	// number is 1250.5 as the language writes it.
	number string
	// detect lists the languages detection may answer with, where close
	// relatives cannot be told apart from two sentences; nil means the
	// language itself.
	detect []string
}

const nb = " "

var samples = []sample{
	{lang: "af", text: "Die kontrak begin vandag. Die huurder betaal elke maand die huur aan die verhuurder, en die deposito is nie betaalbaar nie.", term: "kontrak", form: "al die kontrakte", number: "1" + nb + "250,5"},
	{lang: "sq", text: "Kontrata fillon sot. Qiramarrësi i paguan qiranë qiradhënësit çdo muaj dhe depozita nuk është e pagueshme në këtë muaj.", term: "kontratë", form: "sipas kontratës", number: "1" + nb + "250,5"},
	{lang: "am", text: "ውሉ ዛሬ ይጀምራል። ተከራዩ በየወሩ ኪራዩን ለአከራዩ ይከፍላል፣ ተቀማጩም በዚህ ወር አይከፈልም።", term: "ተከራይ", form: "ተከራዩ በየወሩ", number: "1,250.5"},
	{lang: "ar", text: "يبدأ العقد اليوم. يدفع المستأجر الإيجار إلى المؤجر كل شهر، والوديعة ليست مستحقة في هذا الشهر.", term: "عقد", form: "يبدأ العَقْدُ اليوم", number: "١٬٢٥٠٫٥"},
	{lang: "hy", text: "Պայմանագիրը սկսվում է այսօր։ Վարձակալը ամեն ամիս վճարում է վարձը վարձատուին, և ավանդը այս ամիս ենթակա չէ վճարման։", term: "պայմանագիր", form: "Պայմանագիրը սկսվում է", number: "1" + nb + "250,5"},
	{lang: "az", text: "Müqavilə bu gün başlayır. Kirayəçi hər ay kirayə haqqını icarəyə verənə ödəyir və depozit bu ay üçün ödənilmir.", term: "müqavilə", form: "MÜQAVİLƏNİN şərtləri", number: "1.250,5"},
	{lang: "eu", text: "Kontratua gaur hasten da. Maizterrak hilero ordaintzen dio errenta jabeari, eta fidantza ez da hilabete honetan ordaindu behar.", term: "kontratu", form: "kontratuaren arabera", number: "1.250,5"},
	{lang: "be", text: "Дагавор пачынаецца сёння. Арандатар кожны месяц плаціць арэндную плату арэндадаўцу, і дэпазіт не падлягае выплаце ў гэтым месяцы.", term: "дагавор", form: "умовы дагавора", number: "1" + nb + "250,5"},
	{lang: "bn", text: "চুক্তিটি আজ শুরু হয়। ভাড়াটিয়া প্রতি মাসে বাড়িওয়ালাকে ভাড়া প্রদান করে এবং জামানত এই মাসে প্রদেয় নয়।", term: "চুক্তি", form: "চুক্তিটি আজ", number: "1,250.5"},
	{lang: "bs", text: "Ugovor počinje danas. Zakupac svakog mjeseca plaća zakupninu zakupodavcu, a depozit nije dospio za plaćanje u ovom mjesecu.", term: "ugovor", form: "odredbe ugovora", number: "1.250,5", detect: []string{"hr-HR", "sl-SI"}},
	{lang: "bg", text: "Договорът започва днес. Наемателят плаща наема на наемодателя всеки месец, а депозитът не е дължим за този месец.", term: "договор", form: "Договорът започва", number: "1" + nb + "250,5"},
	{lang: "my", text: "စာချုပ်သည် ယနေ့စတင်သည်။ အိမ်ငှားသည် လစဉ် အိမ်ရှင်ထံ ငှားရမ်းခကို ပေးဆောင်သည်။", term: "စာချုပ်", form: "စာချုပ်သည် ယနေ့", number: "1,250.5"},
	{lang: "ca", text: "El contracte comença avui. El llogater paga el lloguer al propietari cada mes, i la fiança no és exigible aquest mes.", term: "contracte", form: "els contractes", number: "1.250,5"},
	{lang: "zh-CN", text: "合同今天开始。承租人每月向出租人支付租金，押金本月无需支付。", term: "合同", form: "本合同今天开始", number: "1,250.5"},
	{lang: "zh-TW", text: "合約今天開始。承租人每月向出租人支付租金，押金本月無需支付。", term: "合約", form: "本合約今天開始", number: "1,250.5"},
	{lang: "hr", text: "Ugovor počinje danas. Najmoprimac svaki mjesec plaća najamninu najmodavcu, a polog nije dospio za plaćanje u ovom mjesecu.", term: "ugovor", form: "odredbe ugovora", number: "1.250,5", detect: []string{"hr-HR", "sl-SI"}},
	{lang: "cs", text: "Smlouva začíná dnes. Nájemce platí nájemné pronajímateli každý měsíc a kauce není v tomto měsíci splatná.", term: "smlouva", form: "podle smlouvy", number: "1" + nb + "250,5", detect: []string{"cs-CZ", "sk-SK"}},
	{lang: "da", text: "Kontrakten begynder i dag. Lejeren betaler hver måned lejen til udlejeren, og depositummet er ikke forfaldent til betaling.", term: "kontrakt", form: "Kontrakten begynder", number: "1.250,5", detect: []string{"da-DK", "nb-NO"}},
	{lang: "nl", text: "Het contract begint vandaag. De huurder betaalt elke maand de huur aan de verhuurder, en de borg is deze maand niet verschuldigd.", term: "contract", form: "alle contracten", number: "1.250,5"},
	{lang: "en", text: "The contract begins today. The tenant pays the rent to the landlord every month, and the deposit is not due this month.", term: "contract", form: "all contracts", number: "1,250.5"},
	{lang: "et", text: "Leping algab täna. Üürnik maksab üüri üürileandjale iga kuu ja tagatisraha ei ole sel kuul tasumisele kuuluv.", term: "leping", form: "lepingu tingimused", number: "1" + nb + "250,5"},
	{lang: "fil", text: "Ang kontrata ay magsisimula ngayon. Ang nangungupahan ay nagbabayad ng upa sa may-ari bawat buwan, at ang deposito ay hindi pa dapat bayaran.", term: "kontrata", form: "Ang kontrata ay", number: "1,250.5"},
	{lang: "fi", text: "Sopimus alkaa tänään. Vuokralainen maksaa vuokran vuokranantajalle joka kuukausi, ja vakuus ei ole erääntynyt tässä kuussa.", term: "sopimus", form: "sopimuksen ehdot", number: "1" + nb + "250,5"},
	{lang: "fr", text: "Le contrat commence aujourd'hui. Le locataire paie le loyer au bailleur chaque mois, et le dépôt n'est pas exigible ce mois-ci.", term: "contrat", form: "tous les contrats", number: "1 250,5"},
	{lang: "gl", text: "O contrato comeza hoxe. O arrendatario paga a renda ao arrendador cada mes, e a fianza non é esixible este mes.", term: "contrato", form: "todos os contratos", number: "1.250,5", detect: []string{"gl-ES", "pt-PT"}},
	{lang: "ka", text: "ხელშეკრულება დღეს იწყება. დამქირავებელი ყოველთვიურად უხდის ქირას გამქირავებელს, ხოლო დეპოზიტი ამ თვეში გადასახდელი არ არის.", term: "ხელშეკრულება", form: "ხელშეკრულების პირობები", number: "1" + nb + "250,5"},
	{lang: "de", text: "Der Vertrag beginnt heute. Der Mieter zahlt dem Vermieter jeden Monat die Miete, und die Kaution ist in diesem Monat nicht fällig.", term: "Vertrag", form: "alle Verträge", number: "1.250,5"},
	{lang: "el", text: "Η σύμβαση αρχίζει σήμερα. Ο μισθωτής καταβάλλει το μίσθωμα στον εκμισθωτή κάθε μήνα και η εγγύηση δεν είναι απαιτητή αυτόν τον μήνα.", term: "σύμβαση", form: "οι όροι της σύμβασης", number: "1.250,5"},
	{lang: "gu", text: "કરાર આજથી શરૂ થાય છે. ભાડૂત દર મહિને મકાનમાલિકને ભાડું ચૂકવે છે અને ડિપોઝિટ આ મહિને ચૂકવવાપાત્ર નથી.", term: "કરાર", form: "કરારની શરતો", number: "1,250.5"},
	{lang: "he", text: "החוזה מתחיל היום. השוכר משלם את דמי השכירות למשכיר בכל חודש, והפיקדון אינו עומד לפירעון בחודש זה.", term: "חוזה", form: "החוזה מתחיל", number: "1,250.5"},
	{lang: "hi", text: "अनुबंध आज से शुरू होता है। किरायेदार हर महीने मकान मालिक को किराया देता है और जमा राशि इस महीने देय नहीं है।", term: "अनुबंध", form: "सभी अनुबंधों में", number: "१,२५०.५"},
	{lang: "hu", text: "A szerződés ma kezdődik. A bérlő minden hónapban megfizeti a bérleti díjat a bérbeadónak, és a kaució ebben a hónapban nem esedékes.", term: "szerződés", form: "a szerződésben", number: "1" + nb + "250,5"},
	{lang: "is", text: "Samningurinn hefst í dag. Leigjandinn greiðir leiguna til leigusalans í hverjum mánuði og tryggingin er ekki á gjalddaga.", term: "samningur", form: "Samningurinn hefst", number: "1.250,5"},
	{lang: "id", text: "Kontrak dimulai hari ini. Penyewa membayar sewa kepada pemilik setiap bulan, dan deposit tidak jatuh tempo pada bulan ini.", term: "kontrak", form: "isi kontraknya", number: "1.250,5", detect: []string{"id-ID", "ms-MY"}},
	{lang: "ga", text: "Tosaíonn an conradh inniu. Íocann an tionónta an cíos leis an tiarna talún gach mí, agus níl an éarlais dlite an mhí seo.", term: "conradh", form: "an conradh inniu", number: "1,250.5"},
	{lang: "it", text: "Il contratto inizia oggi. Il conduttore paga il canone al locatore ogni mese e il deposito non è esigibile in questo mese.", term: "contratto", form: "tutti i contratti", number: "1.250,5"},
	{lang: "ja", text: "契約は本日開始します。借主は毎月貸主に家賃を支払い、敷金は今月支払う必要はありません。", term: "契約", form: "本契約は本日", number: "1,250.5"},
	{lang: "kn", text: "ಒಪ್ಪಂದವು ಇಂದು ಪ್ರಾರಂಭವಾಗುತ್ತದೆ. ಬಾಡಿಗೆದಾರನು ಪ್ರತಿ ತಿಂಗಳು ಮಾಲೀಕರಿಗೆ ಬಾಡಿಗೆಯನ್ನು ಪಾವತಿಸುತ್ತಾನೆ ಮತ್ತು ಠೇವಣಿ ಈ ತಿಂಗಳು ಪಾವತಿಸಬೇಕಾಗಿಲ್ಲ.", term: "ಒಪ್ಪಂದ", form: "ಒಪ್ಪಂದವು ಇಂದು", number: "1,250.5"},
	{lang: "kk", text: "Шарт бүгін басталады. Жалға алушы ай сайын жалға берушіге жалдау ақысын төлейді және депозит осы айда төленбейді.", term: "шарт", form: "шарттың талаптары", number: "1" + nb + "250,5"},
	{lang: "km", text: "កិច្ចសន្យាចាប់ផ្តើមថ្ងៃនេះ។ អ្នកជួលបង់ថ្លៃឈ្នួលជូនម្ចាស់ផ្ទះរៀងរាល់ខែ។", term: "កិច្ចសន្យា", form: "កិច្ចសន្យាចាប់ផ្តើម", number: "1.250,5"},
	{lang: "ko", text: "계약은 오늘 시작됩니다. 임차인은 매월 임대인에게 임대료를 지급하며 보증금은 이번 달에 지급할 필요가 없습니다.", term: "계약", form: "본 계약은 오늘", number: "1,250.5"},
	{lang: "lo", text: "ສັນຍາເລີ່ມຕົ້ນມື້ນີ້ ຜູ້ເຊົ່າຈ່າຍຄ່າເຊົ່າໃຫ້ເຈົ້າຂອງທຸກເດືອນ", sentences: 1, term: "ສັນຍາ", form: "ສັນຍາເລີ່ມຕົ້ນ", number: "1.250,5"},
	{lang: "lv", text: "Līgums sākas šodien. Īrnieks katru mēnesi maksā īres maksu izīrētājam, un depozīts šajā mēnesī nav jāmaksā.", term: "līgums", form: "līguma noteikumi", number: "1" + nb + "250,5"},
	{lang: "lt", text: "Sutartis prasideda šiandien. Nuomininkas kiekvieną mėnesį moka nuomos mokestį nuomotojui, o užstatas šį mėnesį nėra mokėtinas.", term: "sutartis", form: "sutarties sąlygos", number: "1" + nb + "250,5"},
	{lang: "lb", text: "De Kontrakt fänkt haut un. De Locataire bezilt all Mount de Loyer un de Proprietär, an d'Kautioun ass dëse Mount net fälleg.", term: "Kontrakt", form: "De Kontrakt fänkt", number: "1.250,5"},
	{lang: "mk", text: "Договорот започнува денес. Закупецот секој месец ја плаќа закупнината на закуподавачот, а депозитот не е достасан овој месец.", term: "договор", form: "Договорот започнува", number: "1.250,5"},
	{lang: "ms", text: "Kontrak bermula hari ini. Penyewa membayar sewa kepada tuan rumah setiap bulan, dan deposit tidak perlu dibayar pada bulan ini.", term: "kontrak", form: "isi kontraknya", number: "1,250.5", detect: []string{"id-ID", "ms-MY"}},
	{lang: "ml", text: "കരാർ ഇന്ന് ആരംഭിക്കുന്നു. വാടകക്കാരൻ എല്ലാ മാസവും ഉടമയ്ക്ക് വാടക നൽകുന്നു, നിക്ഷേപം ഈ മാസം നൽകേണ്ടതില്ല.", term: "കരാർ", form: "കരാർ ഇന്ന്", number: "1,250.5"},
	{lang: "mt", text: "Il-kuntratt jibda llum. L-inkwilin iħallas il-kera lil sid il-kera kull xahar, u d-depożitu mhuwiex dovut dan ix-xahar.", term: "kuntratt", form: "Il-kuntratt jibda", number: "1,250.5"},
	{lang: "mr", text: "करार आज सुरू होतो. भाडेकरू दर महिन्याला घरमालकाला भाडे देतो आणि ठेव या महिन्यात देय नाही.", term: "करार", form: "कराराचा कालावधी", number: "1,250.5"},
	{lang: "mn", text: "Гэрээ өнөөдөр эхэлнэ. Түрээслэгч сар бүр түрээсийн төлбөрийг түрээслүүлэгчид төлдөг бөгөөд барьцаа энэ сард төлөгдөхгүй.", term: "гэрээ", form: "гэрээний нөхцөл", number: "1,250.5"},
	{lang: "ne", text: "सम्झौता आज सुरु हुन्छ। भाडावालले हरेक महिना घरधनीलाई भाडा तिर्छ र धरौटी यो महिना तिर्नुपर्दैन।", term: "सम्झौता", form: "सम्झौताको अवधि", number: "1,250.5"},
	{lang: "nb", text: "Kontrakten begynner i dag. Leietakeren betaler leien til utleieren hver måned, og depositumet er ikke forfalt til betaling.", term: "kontrakt", form: "Kontrakten begynner", number: "1" + nb + "250,5", detect: []string{"nb-NO", "da-DK"}},
	{lang: "nn", text: "Kontrakten byrjar i dag. Leigetakaren betaler leiga til utleigaren kvar månad, og depositumet er ikkje forfalle til betaling.", term: "kontrakt", form: "Kontrakten byrjar", number: "1" + nb + "250,5", detect: []string{"nb-NO", "da-DK", "sv-SE"}},
	{lang: "fa", text: "قرارداد امروز آغاز می‌شود. مستأجر هر ماه اجاره را به موجر پرداخت می‌کند و ودیعه در این ماه قابل پرداخت نیست.", term: "قرارداد", form: "همه قراردادها", number: "۱٬۲۵۰٫۵"},
	{lang: "pl", text: "Umowa zaczyna się dzisiaj. Najemca płaci czynsz wynajmującemu co miesiąc, a kaucja nie jest wymagalna w tym miesiącu.", term: "umowa", form: "warunki umowy", number: "1" + nb + "250,5"},
	{lang: "pt", text: "O contrato começa hoje. O inquilino paga a renda ao senhorio todos os meses, e a caução não é exigível neste mês.", term: "contrato", form: "todos os contratos", number: "1.250,5", detect: []string{"pt-PT", "gl-ES"}},
	{lang: "pa", text: "ਇਕਰਾਰਨਾਮਾ ਅੱਜ ਸ਼ੁਰੂ ਹੁੰਦਾ ਹੈ। ਕਿਰਾਏਦਾਰ ਹਰ ਮਹੀਨੇ ਮਕਾਨ ਮਾਲਕ ਨੂੰ ਕਿਰਾਇਆ ਦਿੰਦਾ ਹੈ ਅਤੇ ਜਮ੍ਹਾਂ ਰਕਮ ਇਸ ਮਹੀਨੇ ਦੇਣਯੋਗ ਨਹੀਂ ਹੈ।", term: "ਇਕਰਾਰਨਾਮਾ", form: "ਇਕਰਾਰਨਾਮੇ ਦੀਆਂ ਸ਼ਰਤਾਂ", number: "1,250.5"},
	{lang: "ro", text: "Contractul începe astăzi. Chiriașul plătește chiria proprietarului în fiecare lună, iar garanția nu este scadentă în această lună.", term: "contract", form: "Contractul începe", number: "1.250,5"},
	{lang: "ru", text: "Договор начинается сегодня. Арендатор ежемесячно платит арендную плату арендодателю, и депозит не подлежит уплате в этом месяце.", term: "договор", form: "условия договора", number: "1" + nb + "250,5"},
	{lang: "sr-Cyrl-RS", text: "Уговор почиње данас. Закупац сваког месеца плаћа закупнину закуподавцу, а депозит није доспео за плаћање у овом месецу.", term: "уговор", form: "одредбе уговора", number: "1.250,5"},
	{lang: "sr-Latn-RS", text: "Ugovor počinje danas. Zakupac svakog meseca plaća zakupninu zakupodavcu, a depozit nije dospeo za plaćanje u ovom mesecu.", term: "ugovor", form: "odredbe ugovora", number: "1.250,5", detect: []string{"hr-HR", "sl-SI"}},
	{lang: "si", text: "ගිවිසුම අද ආරම්භ වේ. කුලී නිවැසියා සෑම මසකම හිමිකරුට කුලිය ගෙවයි, තැන්පතුව මෙම මාසයේ ගෙවිය යුතු නැත.", term: "ගිවිසුම", form: "ගිවිසුමේ කොන්දේසි", number: "1,250.5"},
	{lang: "sk", text: "Zmluva začína dnes. Nájomca platí nájomné prenajímateľovi každý mesiac a kaucia nie je v tomto mesiaci splatná.", term: "zmluva", form: "podľa zmluvy", number: "1" + nb + "250,5", detect: []string{"sk-SK", "cs-CZ"}},
	{lang: "sl", text: "Pogodba se začne danes. Najemnik vsak mesec plača najemnino najemodajalcu, varščina pa ta mesec ni zapadla v plačilo.", term: "pogodba", form: "določbe pogodbe", number: "1.250,5", detect: []string{"sl-SI", "hr-HR"}},
	{lang: "es", text: "El contrato comienza hoy. El arrendatario paga la renta al arrendador cada mes, y la fianza no es exigible este mes.", term: "contrato", form: "todos los contratos", number: "1.250,5"},
	{lang: "es-MX", text: "El contrato comienza hoy. El arrendatario paga la renta al arrendador cada mes, y el depósito no es exigible este mes.", term: "contrato", form: "todos los contratos", number: "1,250.5", detect: []string{"es-ES"}},
	{lang: "sw", text: "Mkataba unaanza leo. Mpangaji hulipa kodi kwa mwenye nyumba kila mwezi, na amana hailipwi katika mwezi huu.", term: "mkataba", form: "Mkataba unaanza", number: "1,250.5"},
	{lang: "sv", text: "Avtalet börjar i dag. Hyresgästen betalar hyran till hyresvärden varje månad, och depositionen är inte förfallen till betalning.", term: "avtal", form: "Avtalet börjar", number: "1" + nb + "250,5"},
	{lang: "ta", text: "ஒப்பந்தம் இன்று தொடங்குகிறது. குத்தகைதாரர் ஒவ்வொரு மாதமும் உரிமையாளருக்கு வாடகை செலுத்துகிறார், வைப்புத்தொகை இந்த மாதம் செலுத்த வேண்டியதில்லை.", term: "ஒப்பந்தம்", form: "ஒப்பந்தத்தின் விதிமுறைகள்", number: "1,250.5"},
	{lang: "te", text: "ఒప్పందం ఈ రోజు ప్రారంభమవుతుంది. అద్దెదారు ప్రతి నెల యజమానికి అద్దె చెల్లిస్తాడు మరియు డిపాజిట్ ఈ నెలలో చెల్లించవలసిన అవసరం లేదు.", term: "ఒప్పందం", form: "ఒప్పందంలో", number: "1,250.5"},
	{lang: "th", text: "สัญญาเริ่มต้นวันนี้ ผู้เช่าชำระค่าเช่าให้แก่ผู้ให้เช่าทุกเดือน และเงินมัดจำยังไม่ถึงกำหนดชำระ", sentences: 1, term: "สัญญา", form: "สัญญาเริ่มต้น", number: "๑,๒๕๐.๕"},
	{lang: "tr", text: "Sözleşme bugün başlar. Kiracı her ay kirayı kiraya verene öder ve depozito bu ay için muaccel değildir.", term: "sözleşme", form: "SÖZLEŞMENİN şartları", number: "1.250,5"},
	{lang: "uk", text: "Угода починається сьогодні. Орендар щомісяця сплачує орендну плату орендодавцю, і депозит не підлягає сплаті в цьому місяці.", term: "угода", form: "умови угоди", number: "1" + nb + "250,5"},
	{lang: "ur", text: "معاہدہ آج شروع ہوتا ہے۔ کرایہ دار ہر ماہ مالک مکان کو کرایہ ادا کرتا ہے اور زر ضمانت اس ماہ واجب الادا نہیں ہے۔", term: "معاہدہ", form: "معاہدے کی شرائط", number: "1,250.5"},
	{lang: "uz", text: "Shartnoma bugun boshlanadi. Ijarachi har oy ijara haqini ijaraga beruvchiga to'laydi va depozit bu oyda to'lanmaydi.", term: "shartnoma", form: "shartnomaning shartlari", number: "1" + nb + "250,5"},
	{lang: "vi", text: "Hợp đồng bắt đầu từ hôm nay. Người thuê trả tiền thuê cho chủ nhà hàng tháng và tiền đặt cọc không phải trả trong tháng này.", term: "hợp đồng", form: "Hợp đồng bắt đầu", number: "1.250,5"},
	{lang: "cy", text: "Mae'r contract yn dechrau heddiw. Mae'r tenant yn talu'r rhent i'r landlord bob mis, ac nid yw'r blaendal yn ddyledus y mis hwn.", term: "contract", form: "Mae'r contract yn", number: "1,250.5"},
}
