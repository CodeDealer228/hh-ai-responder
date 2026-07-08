package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type Vacancy struct {
	ID                     int               `json:"vacancyId"`
	Name                   string            `json:"name"`
	WorkSchedule           string            `json:"@workSchedule"`
	Links                  map[string]string `json:"links"`
	TotalResponsesCount    int               `json:"totalResponsesCount"`
	Area                   NamedObject       `json:"area"`
	Company                Company           `json:"company"`
	Compensation           Compensation      `json:"compensation"`
	CreationTime           string            `json:"creationTime"`
	LastChangeTime         ChangeTime        `json:"lastChangeTime"`
	UserLabels             []string          `json:"userLabels"`
	ResponseLetterRequired bool              `json:"@responseLetterRequired"`
	UserTestPresent        bool              `json:"userTestPresent"`
	Archived               bool              `json:"archived"`
	ResponseURL            string            `json:"response_url"`
}

type NamedObject struct {
	Name string `json:"name"`
}

type Company struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	CompanySiteURL string `json:"companySiteUrl"`
}

// type Compensation struct {
// 	From         int    `json:"from"`
// 	To           int    `json:"to"`
// 	CurrencyCode string `json:"currencyCode"`
// }

type ChangeTime struct {
	Value string `json:"$"`
}

type VacancyTest struct {
	UIDPk       string `json:"uidPk"`
	GUID        string `json:"guid"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    string `json:"required"`
	StartTime   string `json:"startTime"`
	Tasks       []Task `json:"tasks"`
}

type Task struct {
	ID                 int        `json:"id"`
	Description        string     `json:"description"`
	Multiple           string     `json:"multiple"`
	Open               string     `json:"open"`
	CandidateSolutions []Solution `json:"candidateSolutions"`
}

type Solution struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Title string `json:"title"`
	Value string `json:"value"`
}

type SolutionFields struct {
	SolutionID   int
	TextSolution string
	HasChoice    bool
}

type ApplyResult struct {
	Type           string    `json:"type"`
	Resume         string    `json:"resume"`
	ResumeTitle    string    `json:"resume_title"`
	VacancyID      int       `json:"vacancy_id"`
	URL            string    `json:"url"`
	Name           string    `json:"name"`
	Letter         string    `json:"letter"`
	AppliedAt      time.Time `json:"applied_at"`
	ResponsesCount int       `json:"responses_count"`
	TestSolutions  []QAPair  `json:"test_solutions,omitempty"`
}

type ChatResult struct {
	Type        string    `json:"type"`
	Resume      string    `json:"resume"`
	ResumeTitle string    `json:"resume_title"`
	ChatId      int64     `json:"chat_id"`
	EmployerMsg string    `json:"employer_message"`
	Reply       string    `json:"reply"`
	SentAt      time.Time `json:"sent_at"`
}

type ResumeTouchResult struct {
	Type        string    `json:"type"`
	Resume      string    `json:"resume"`
	ResumeTitle string    `json:"resume_title"`
	Updated     bool      `json:"updated"`
	Time        time.Time `json:"time"`
}

type ErrorResult struct {
	Type    string         `json:"type"`
	Context map[string]any `json:"context"`
	Error   string         `json:"error"`
	Time    time.Time      `json:"time"`
}

type QAPair struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// ===== Chat API Types =====
type ChatsResponse struct {
	Chats            ChatsList                  `json:"chats"`
	ChatsDisplayInfo map[string]ChatDisplayInfo `json:"chatsDisplayInfo"`
	Resources        ChatsResources             `json:"resources"`
}

type ChatsList struct {
	Page    int            `json:"page"`
	PerPage int            `json:"per_page"`
	Pages   int            `json:"pages"`
	Items   []ChatListItem `json:"items"`
}

type ChatListItem struct {
	Id                               int64             `json:"id"`
	Type                             string            `json:"type"`
	SubType                          interface{}       `json:"subType"`
	UnreadCount                      int               `json:"unreadCount"`
	Resources                        ChatItemResources `json:"resources"`
	Pinned                           bool              `json:"pinned"`
	NotificationEnabled              bool              `json:"notificationEnabled"`
	OwnerViolatesRules               bool              `json:"ownerViolatesRules"`
	CurrentParticipantID             string            `json:"currentParticipantId"`
	LastMessage                      *ChatMessage      `json:"lastMessage,omitempty"`
	LastViewedByOpponentMessageID    int64             `json:"lastViewedByOpponentMessageId"`
	LastViewedByCurrentUserMessageID *int64            `json:"lastViewedByCurrentUserMessageId"`
	ParticipantsIDs                  []string          `json:"participantsIds"`
	OnlineUntilTime                  *time.Time        `json:"onlineUntilTime"`
	LastActivityTime                 time.Time         `json:"lastActivityTime"`
}

type ChatDataResponse struct {
	Chat             ChatDetail        `json:"chat"`
	Resources        ExtendedResources `json:"resources"`
	MissingResources MissingResources  `json:"missingResources"`
	Display          ChatDisplay       `json:"display"`
	ChatStates       ChatStates        `json:"chatStates"`
	Suggestions      Suggestions       `json:"suggestions"`
	HasButtons       bool              `json:"hasMessagesWithTextButtons"`
	CallAvailable    bool              `json:"callAvailable"`
	TopicStates      interface{}       `json:"negotiationTopicsAvailableStates"`
}

type ChatDetail struct {
	ID                               int64             `json:"id"`
	Type                             string            `json:"type"`
	SubType                          interface{}       `json:"subType"`
	BlockInfo                        interface{}       `json:"blockInfo"`
	UnreadCount                      int               `json:"unreadCount"`
	Resources                        ChatItemResources `json:"resources"`
	Pinned                           bool              `json:"pinned"`
	NotificationEnabled              bool              `json:"notificationEnabled"`
	WritePossibility                 WritePossibility  `json:"writePossibility"`
	Operations                       Operations        `json:"operations"`
	OwnerViolatesRules               bool              `json:"ownerViolatesRules"`
	Messages                         ChatMessages      `json:"messages"`
	CurrentParticipantID             string            `json:"currentParticipantId"`
	LastViewedByOpponentMessageID    int64             `json:"lastViewedByOpponentMessageId"`
	LastViewedByCurrentUserMessageID *int64            `json:"lastViewedByCurrentUserMessageId"`
	ParticipantsIDs                  []string          `json:"participantsIds"`
	OnlineUntilTime                  *time.Time        `json:"onlineUntilTime"`
	LastActivityTime                 time.Time         `json:"lastActivityTime"`
}

type WritePossibility struct {
	Name                 string   `json:"name"`
	WriteDisabledReasons []string `json:"writeDisabledReasons"`
}

type Operations struct {
	Enabled []string `json:"enabled"`
}

type ChatItemResources struct {
	Vacancy          []string `json:"VACANCY"`
	NegotiationTopic []string `json:"NEGOTIATION_TOPIC"`
	Resume           []string `json:"RESUME"`
	Unknown          []string `json:"UNKNOWN"`
}

type ChatMessages struct {
	Items   []ChatMessage `json:"items"`
	HasMore bool          `json:"hasMore"`
}

type ChatMessage struct {
	ID                   int64               `json:"id"`
	ChatID               int64               `json:"chatId"`
	CreationTime         time.Time           `json:"creationTime"`
	Text                 string              `json:"text"`
	Resources            ChatItemResources   `json:"resources,omitempty"`
	Type                 string              `json:"type"`
	CanEdit              bool                `json:"canEdit"`
	CanDelete            bool                `json:"canDelete"`
	WorkflowTransitionID int64               `json:"workflowTransitionId"`
	OnlyVisibleForMyType bool                `json:"onlyVisibleForMyType"`
	Flags                MessageFlags        `json:"flags"`
	HasContent           bool                `json:"hasContent"`
	Hidden               bool                `json:"hidden"`
	WorkflowTransition   *WorkflowTransition `json:"workflowTransition"`
	ParticipantDisplay   ParticipantDisplay  `json:"participantDisplay"`
	ParticipantID        string              `json:"participantId"`
	Actions              *MessageActions     `json:"actions,omitempty"`
}

type MessageActions struct {
	TextButtons []TextButton `json:"text_buttons"`
}

type TextButton struct {
	Size string `json:"size"`
	Text string `json:"text"`
}

type MessageFlags struct {
	ShouldCheckLinks bool `json:"shouldCheckLinks"`
}

type WorkflowTransition struct {
	ID                  int64  `json:"id"`
	TopicID             int64  `json:"topicId"`
	ApplicantState      string `json:"applicantState"`
	DeclinedByApplicant bool   `json:"declinedByApplicant"`
}

type ParticipantDisplay struct {
	Name   string `json:"name"`
	IsBot  bool   `json:"isBot"`
	Avatar string `json:"avatar,omitempty"`
}

type ExtendedResources struct {
	Vacancies         map[string]ChatDetailVacancy `json:"vacancies"`
	Employers         map[string]interface{}       `json:"employers"`
	Resumes           map[string]Resume            `json:"resumes"`
	ResumeHashById    map[string]string            `json:"resumeHashById"`
	Participants      map[string]ParticipantDetail `json:"participants"`
	NegotiationTopics map[string]NegotiationTopic  `json:"negotiation_topics"`
	Addresses         map[string]interface{}       `json:"addresses"`
	TestSolutions     map[string]interface{}       `json:"test_solutions"`
	FileInfoByUpload  map[string]interface{}       `json:"file_info_by_upload_ids"`
	EmployerAssistant map[string]interface{}       `json:"employer_assistant"`
}

type ChatDetailVacancy struct {
	WorkSchedule            string             `json:"@workSchedule"`
	ShowContact             bool               `json:"@showContact"`
	ResponseLetterRequired  bool               `json:"@responseLetterRequired"`
	VacancyID               int64              `json:"vacancyId"`
	Name                    string             `json:"name"`
	Company                 ChatDetailCompany  `json:"company"`
	Compensation            Compensation       `json:"compensation"`
	PublicationTime         CustomTime         `json:"publicationTime"`
	Area                    Area               `json:"area"`
	AcceptTemporary         bool               `json:"acceptTemporary"`
	CreationSite            string             `json:"creationSite"`
	CreationSiteID          int                `json:"creationSiteId"`
	DisplayHost             string             `json:"displayHost"`
	LastChangeTime          CustomTime         `json:"lastChangeTime"`
	CreationTime            time.Time          `json:"creationTime"`
	CanBeShared             bool               `json:"canBeShared"`
	EmployerManager         interface{}        `json:"employerManager"`
	InboxPossibility        bool               `json:"inboxPossibility"`
	ChatWritePossibility    string             `json:"chatWritePossibility"`
	Notify                  bool               `json:"notify"`
	Links                   Links              `json:"links"`
	AcceptIncompleteResumes bool               `json:"acceptIncompleteResumes"`
	DriverLicenseTypes      []interface{}      `json:"driverLicenseTypes"`
	Languages               []interface{}      `json:"languages"`
	WorkingDays             []interface{}      `json:"workingDays"`
	WorkingTimeIntervals    []interface{}      `json:"workingTimeIntervals"`
	WorkingTimeModes        []interface{}      `json:"workingTimeModes"`
	VacancyProperties       VacancyProperties  `json:"vacancyProperties"`
	VacancyPlatforms        []string           `json:"vacancyPlatforms"`
	ProfessionalRoleIds     []ProfessionalRole `json:"professionalRoleIds"`
	WorkExperience          string             `json:"workExperience"`
	Employment              Employment         `json:"employment"`
	ClosedForApplicants     bool               `json:"closedForApplicants"`
	UserTestPresent         bool               `json:"userTestPresent"`
	EmploymentForm          string             `json:"employmentForm"`
	FlyInFlyOutDurations    []interface{}      `json:"flyInFlyOutDurations"`
	Internship              bool               `json:"internship"`
	NightShifts             bool               `json:"nightShifts"`
	WorkFormats             []WorkFormat       `json:"workFormats"`
	WorkScheduleByDays      []WorkScheduleDays `json:"workScheduleByDays"`
	WorkingHours            []WorkingHours     `json:"workingHours"`
	ExperimentalModes       []ExperimentalMode `json:"experimentalModes"`
	AcceptLaborContract     bool               `json:"acceptLaborContract"`
	CivilLawContracts       []interface{}      `json:"civilLawContracts"`
	AutoResponse            AutoResponse       `json:"autoResponse"`
	InclusivenessTypes      []interface{}      `json:"inclusivenessTypes"`
}

type ChatDetailCompany struct {
	ShowSimilarVacancies      bool   `json:"@showSimilarVacancies"`
	Trusted                   bool   `json:"@trusted"`
	Category                  string `json:"@category"`
	CountryID                 int    `json:"@countryId"`
	State                     string `json:"@state"`
	ID                        int64  `json:"id"`
	Name                      string `json:"name"`
	VisibleName               string `json:"visibleName"`
	Logos                     Logos  `json:"logos"`
	EmployerOrganizationForm  int    `json:"employerOrganizationFormId"`
	ShowOrganizationForm      bool   `json:"showOrganizationForm"`
	Badges                    Badges `json:"badges"`
	CompanySiteURL            string `json:"companySiteUrl"`
	AccreditedITEmployer      bool   `json:"accreditedITEmployer"`
	EmployerOnAdditionalCheck bool   `json:"employerOnAdditionalCheck"`
}

type Logos struct {
	Logo []LogoItem `json:"logo"`
}

type LogoItem struct {
	Type string `json:"@type"`
	URL  string `json:"@url"`
}

type Badges struct {
	Badge []BadgeItem `json:"badge"`
}

type BadgeItem struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// type Compensation struct {
// 	NoCompensation interface{} `json:"noCompensation"`
// }

type CustomTime struct {
	Timestamp int64     `json:"@timestamp"`
	Value     time.Time `json:"$"`
}

type Area struct {
	ID   int64  `json:"@id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type Links struct {
	Desktop string `json:"desktop"`
	Mobile  string `json:"mobile"`
}

type VacancyProperties struct {
	Properties       []PropertyBlock  `json:"properties"`
	CalculatedStates CalculatedStates `json:"calculatedStates"`
}

type PropertyBlock struct {
	Property []PropertyItem `json:"property"`
}

type PropertyItem struct {
	ID             int64        `json:"id"`
	PropertyType   string       `json:"propertyType"`
	Defining       bool         `json:"defining,omitempty"`
	Classifying    bool         `json:"classifying,omitempty"`
	Bundle         string       `json:"bundle"`
	PropertyWeight int          `json:"propertyWeight"`
	Parameters     []ParamBlock `json:"parameters"`
	StartTimeIso   time.Time    `json:"startTimeIso"`
	EndTimeIso     time.Time    `json:"endTimeIso"`
}

type ParamBlock struct {
	Parameter []ParamItem `json:"parameter"`
}

type ParamItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type CalculatedStates struct {
	HH StateDetail `json:"HH"`
	ZP StateDetail `json:"ZP"`
}

type StateDetail struct {
	Advertising           bool     `json:"advertising"`
	Anonymous             bool     `json:"anonymous"`
	CrosspostedTo         []string `json:"crosspostedTo,omitempty"`
	CrosspostedFrom       string   `json:"crosspostedFrom,omitempty"`
	FilteredPropertyNames []string `json:"filteredPropertyNames"`
	Free                  bool     `json:"free"`
	Optimum               bool     `json:"optimum"`
	OptionPremium         bool     `json:"optionPremium"`
	PayForPerformance     bool     `json:"payForPerformance"`
	Premium               bool     `json:"premium"`
	Standard              bool     `json:"standard"`
	StandardPlus          bool     `json:"standardPlus"`
	TranslationKeys       []string `json:"translationKeys"`
}

type ProfessionalRole struct {
	ProfessionalRoleId []int `json:"professionalRoleId"`
}

type Employment struct {
	Type string `json:"@type"`
}

type WorkFormat struct {
	WorkFormatsElement []string `json:"workFormatsElement"`
}

type WorkScheduleDays struct {
	WorkScheduleByDaysElement []string `json:"workScheduleByDaysElement"`
}

type WorkingHours struct {
	WorkingHoursElement []string `json:"workingHoursElement"`
}

type ExperimentalMode struct {
	ExperimentalMode []string `json:"experimentalMode"`
}

type AutoResponse struct {
	AcceptAutoResponse bool `json:"acceptAutoResponse"`
}

type Resume struct {
	Hash         string      `json:"hash"`
	ID           int64       `json:"id"`
	UserID       int64       `json:"userId"`
	Title        string      `json:"title"`
	HiddenFields []string    `json:"hiddenFields"`
	Gender       string      `json:"gender"`
	Phone        []PhoneItem `json:"phone"`
}

type PhoneItem struct {
	Type             string      `json:"type"`
	Country          string      `json:"country"`
	City             string      `json:"city"`
	Number           string      `json:"number"`
	Formatted        string      `json:"formatted"`
	Raw              string      `json:"raw"`
	Verified         bool        `json:"verified"`
	NeedVerification bool        `json:"needVerification"`
	Comment          interface{} `json:"comment"`
}

type ParticipantDetail struct {
	ID               int64         `json:"id"`
	ExternalID       string        `json:"externalId"`
	Type             string        `json:"type"`
	IsCurrentUser    bool          `json:"isCurrentUser"`
	Key              string        `json:"key"`
	Display          UserDisplay   `json:"display"`
	EmployerManager  int64         `json:"employerManagerId,omitempty"`
	LastActivityTime DateTimeBlock `json:"lastActivityTime"`
	OnlineUntilTime  DateTimeBlock `json:"onlineUntilTime"`
	EntityID         int64         `json:"entityId"`
	Role             UserRole      `json:"role"`
}

type UserDisplay struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
}

type DateTimeBlock struct {
	DateTime time.Time `json:"dt"`
}

type UserRole struct {
	Name    string `json:"name"`
	Display string `json:"display"`
}

type NegotiationTopic struct {
	TopicID               int64  `json:"topicId"`
	VacancyID             int64  `json:"vacancyId"`
	ResumeID              int64  `json:"resumeId"`
	InitialTopicType      string `json:"initialTopicType"`
	CurrentTopicType      string `json:"currentTopicType"`
	InitialApplicantState string `json:"initialApplicantState"`
	CurrentApplicantState string `json:"currentApplicantState"`
}

type MissingResources struct {
	Vacancies         interface{} `json:"vacancies"`
	Employers         interface{} `json:"employers"`
	Resumes           interface{} `json:"resumes"`
	Participants      interface{} `json:"participants"`
	NegotiationTopics interface{} `json:"negotiation_topics"`
	Addresses         interface{} `json:"addresses"`
	TestSolutions     interface{} `json:"test_solutions"`
	FileUploadIDs     interface{} `json:"file_upload_ids"`
}

type ChatDisplay struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Icon     string `json:"icon"`
}

type ChatStates struct {
	WriteMessageState     StateAllowed `json:"writeMessageState"`
	ResponseReminderState StateAllowed `json:"responseReminderState"`
	SendFileState         StateAllowed `json:"sendFileState"`
}

type StateAllowed struct {
	Allowed bool        `json:"allowed"`
	Reasons []string    `json:"reasons,omitempty"`
	Reason  interface{} `json:"reason,omitempty"`
}

type Suggestions struct {
	UUID              string            `json:"uuid"`
	SuggestionOptions SuggestionOptions `json:"suggestionOptions"`
}

type SuggestionOptions struct {
	Options []interface{} `json:"options"`
}

type ChatDisplayInfo struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Icon     string `json:"icon,omitempty"`
}

type ChatsResources struct {
	Vacancies           map[string]ChatVacancyResource      `json:"vacancies"`
	Employers           map[string]json.RawMessage          `json:"employers"`
	Resumes             map[string]ChatResumeResource       `json:"resumes"`
	ResumeHashById      map[string]string                   `json:"resumeHashById"`
	Participants        map[string]json.RawMessage          `json:"participants"`
	NegotiationTopics   map[string]ChatNegotiationTopic     `json:"negotiation_topics"`
	Addresses           map[string]json.RawMessage          `json:"addresses"`
	TestSolutions       map[string]ChatTestSolutionResource `json:"test_solutions"`
	FileInfoByUploadIds map[string]json.RawMessage          `json:"file_info_by_upload_ids"`
	EmployerAssistant   map[string]json.RawMessage          `json:"employer_assistant"`
}

type ChatResumeResource struct {
	Area             int           `json:"area"`
	LastName         string        `json:"lastName"`
	FieldsViewStatus *string       `json:"fieldsViewStatus"`
	PhotoUrls        ChatPhotoUrls `json:"photoUrls"`
	Gender           string        `json:"gender"`
	Permission       string        `json:"permission"`
	Salary           *ChatSalary   `json:"salary"`
	Title            string        `json:"title"`
	UserId           int64         `json:"userId"`
	AccessType       string        `json:"accessType"`
	FirstName        string        `json:"firstName"`
	HiddenFields     []string      `json:"hiddenFields"`
	Phone            []ChatPhone   `json:"phone"`
	MiddleName       string        `json:"middleName"`
	Id               int64         `json:"id"`
	Hash             string        `json:"hash"`
	TopicVacancyId   int64         `json:"topic_vacancy_id"`
}

type ChatPhotoUrls struct {
	Id      int64   `json:"id"`
	State   string  `json:"state"`
	Title   *string `json:"title"`
	Big     string  `json:"big"`
	Large   string  `json:"large"`
	Preview string  `json:"preview"`
	Avatar  string  `json:"avatar"`
}

type ChatSalary struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type ChatPhone struct {
	Type             string  `json:"type"`
	Country          string  `json:"country"`
	City             string  `json:"city"`
	Number           string  `json:"number"`
	Formatted        string  `json:"formatted"`
	Raw              string  `json:"raw"`
	Verified         bool    `json:"verified"`
	NeedVerification bool    `json:"needVerification"`
	Comment          *string `json:"comment"`
}

type ChatNegotiationTopic struct {
	TopicId               int64  `json:"topicId"`
	VacancyId             int64  `json:"vacancyId"`
	ResumeId              int64  `json:"resumeId"`
	InitialTopicType      string `json:"initialTopicType"`
	CurrentTopicType      string `json:"currentTopicType"`
	InitialApplicantState string `json:"initialApplicantState"`
	CurrentApplicantState string `json:"currentApplicantState"`
}

type ChatTestSolutionResource struct {
	UidPk    int64  `json:"uidPk"`
	Score    int    `json:"score"`
	Mark     string `json:"mark"`
	Examined bool   `json:"examined"`
}

type ChatVacancyResource struct {
	VacancyID int64  `json:"vacancyId"`
	Name      string `json:"name"`

	Company struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		SiteURL string `json:"companySiteUrl,omitempty"`
		Trusted bool   `json:"trusted,omitempty"`
	} `json:"company"`
	Links        VacancyLinks  `json:"links"`
	Compensation *Compensation `json:"compensation,omitempty"`
}

type VacancyLinks struct {
	Desktop string `json:"desktop"`
	Mobile  string `json:"mobile"`
}

// HH иногда отдаёт разные формы зарплаты
type Compensation struct {
	From     *int   `json:"from,omitempty"`
	To       *int   `json:"to,omitempty"`
	Currency string `json:"currencyCode,omitempty"`
	Gross    *bool  `json:"gross,omitempty"`

	// если нет зарплаты (noCompensation)
	Raw any `json:"-"`
}

func FormatCompensation(c *Compensation) string {
	if c == nil {
		return ""
	}
	if c.Raw != nil && c.From == nil && c.To == nil {
		return ""
	}

	var fromStr, toStr string

	if c.From != nil {
		fromStr = fmt.Sprintf("%d", *c.From)
	}
	if c.To != nil {
		toStr = fmt.Sprintf("%d", *c.To)
	}

	cur := strings.TrimSpace(c.Currency)

	switch {
	case c.From != nil && c.To != nil:
		return fmt.Sprintf("%s-%s %s", fromStr, toStr, cur)

	case c.From != nil && c.To == nil:
		// "от X"
		return fmt.Sprintf("%s+ %s", fromStr, cur)

	case c.From == nil && c.To != nil:
		// "до Y"
		return fmt.Sprintf("0-%s %s", toStr, cur)

	default:
		return ""
	}
}

type HHResponse struct {
	Status int
	URL    *url.URL
	Body   []byte
}

type ChatToReply struct {
	ChatId              int64
	ContactName         string
	ReplyToMessage      string
	VacancyName         string
	VacancyURL          string
	CompanyName         string
	VacancyCompensation string
	ReplyOptions        []string
	ResumeID            int64
	ResumeHash          string
	ResumeTitle         string
	ResumeExperience    string
	ApplicantId         int64
	FirstName           string
	LastName            string
	Salary              string
	Skills              string
	IsDiscard           bool
}

type AccountInfo struct {
	FirstName  string `json:"firstName"`
	MiddleName string `json:"middleName"`
	LastName   string `json:"lastName"`
	Email      string `json:"email"`
}

type ResumeTitle struct {
	String string `json:"string"`
}

type ResumeItem struct {
	Id     int64
	Hash   string
	Title  string
	Skills string
	Area   string
	Salary string
}
