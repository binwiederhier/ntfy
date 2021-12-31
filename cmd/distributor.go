package cmd

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"heckel.io/ntfy/client"

	distributor_tools "unifiedpush.org/go/np2p_dbus/distributor"
	"unifiedpush.org/go/np2p_dbus/storage"
	"unifiedpush.org/go/np2p_dbus/utils"
)

type store struct {
	*storage.Storage
}

func (s store) GetAllPubTokens() string {
	var conns []storage.Connection
	result := s.DB().Find(&conns)
	if result.Error != nil {
		//TODO
	}
	pubtokens := []string{}
	for _, conn := range conns {
		pubtokens = append(pubtokens, conn.PublicToken)
	}

	return strings.Join(pubtokens, ",")
}

type kvStore struct {
	Key   string `gorm:"primaryKey"`
	Value string
}

func (kv *kvStore) get(db *gorm.DB) error {
	return db.First(kv).Error
}

func (kv kvStore) set(db *gorm.DB) error {
	return db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&kv).Error
}

func (s store) SetLastMessage(id int64) error {
	return kvStore{"device-id", fmt.Sprintf("%d", id)}.set(s.DB())
}

func (s store) GetLastMessage() int64 {
	answer := kvStore{Key: "device-id"}
	if err := answer.get(s.DB()); err != nil {
		//log or fatal??
		return 100
	}
	time, _ := strconv.Atoi(answer.Value)
	return int64(time)
}

// creates a new distributor object with an initialized storage and dbus
func newDistributor(conf *client.Config) (d *distributor) {
	st, err := storage.InitStorage(utils.StoragePath("ntfy.db"))
	st.DB().AutoMigrate(kvStore{}) //todo move to proper function
	if err != nil {
		log.Fatalln("failed to connect database")
	}

	dbus := distributor_tools.NewDBus("org.unifiedpush.Distributor.ntfy")
	d = &distributor{dbus, store{st}, conf, make(chan struct{})}
	err = dbus.StartHandling(d)
	fmt.Println("DBUS HANDLING")
	if err != nil {
		log.Fatalln("failed to connect to dbus")
	}

	return
}

type distributor struct {
	dbus  *distributor_tools.DBus
	st    store
	conf  *client.Config
	resub chan struct{}
}

// handleEndpointSettingsChanges runs on every start and
// checks if the new configuration server is different from previously.
// If so, it re-registers the apps which have the old info.
func (d distributor) handleEndpointSettingsChanges() {
	endpointFormat := d.fillInURL("<token>")
	for _, i := range d.st.GetUnequalSettings(endpointFormat) {
		utils.Log.Debugln("new endpoint format for", i.AppID, i.AppToken)
		//newconnection updates the endpoint settings when one already exists
		n := d.st.NewConnection(i.AppID, i.AppToken, endpointFormat)
		if n == nil || n.Settings != endpointFormat {
			utils.Log.Debugln("unable to save new endpoint format for", i.AppID, i.AppToken)
			continue
		}
		d.dbus.NewConnector(n.AppID).NewEndpoint(n.AppToken, d.fillInURL(n.PublicToken))
	}
	d.resub <- struct{}{}
}

// Register handles an app's call to register for a new connection
// this creates a new connection in the db and triggers a resubscribe with that id
// then it returns the endpoint with that new token to dbus
func (d distributor) Register(appName, token string) (string, string, error) {
	fmt.Println(appName, "registration request")
	conn := d.st.NewConnection(appName, token, d.fillInURL("<token>"))
	fmt.Println("registered", conn)
	if conn != nil {
		fmt.Println("F1")
		d.resub <- struct{}{}
		fmt.Println("F2")
		return d.fillInURL(conn.PublicToken), "", nil
	}
	//np2p doesn't have a situation for refuse
	return "", "", errors.New("Unknown error with NoProvider2Push")
}

// Unregister handles an app's unregister request
// It deletes the connection in the database and triggers a resubscribe
func (d distributor) Unregister(token string) {
	deletedConn, err := d.st.DeleteConnection(token)
	utils.Log.Debugln("deleted", deletedConn)

	if err != nil {
		//?????
		return
	}
	_ = d.dbus.NewConnector(deletedConn.AppID).Unregistered(deletedConn.AppToken)

	d.resub <- struct{}{}

}

// Fills in the default host to and app token to make UnifiedPush endpoints
func (d distributor) fillInURL(token string) string {
	return fmt.Sprintf("%s/%s?up=1", d.conf.DefaultHost, token)
}
