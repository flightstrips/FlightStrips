using Microsoft.EntityFrameworkCore.Migrations;
using Npgsql.EntityFrameworkCore.PostgreSQL.Metadata;

#nullable disable

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Migrations
{
    /// <inheritdoc />
    public partial class Initial : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "OnlinePositions",
                columns: table => new
                {
                    Session = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Airport = table.Column<string>(type: "character varying(4)", maxLength: 4, nullable: false),
                    PositionName = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    PositionFrequency = table.Column<string>(type: "character varying(7)", maxLength: 7, nullable: false),
                    FromPlugin = table.Column<bool>(type: "boolean", nullable: false),
                    ConnectedWithUi = table.Column<bool>(type: "boolean", nullable: false),
                    ArrivalRunway = table.Column<string>(type: "character varying(3)", maxLength: 3, nullable: true),
                    DepartureRunway = table.Column<string>(type: "character varying(3)", maxLength: 3, nullable: true),
                    Sector = table.Column<int>(type: "integer", nullable: false),
                    xmin = table.Column<uint>(type: "xid", rowVersion: true, nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_OnlinePositions", x => new { x.PositionName, x.Session, x.Airport });
                });

            migrationBuilder.CreateTable(
                name: "RunwayConfigs",
                columns: table => new
                {
                    Airport = table.Column<string>(type: "character varying(4)", maxLength: 4, nullable: false),
                    Session = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Departure = table.Column<string>(type: "character varying(4)", maxLength: 4, nullable: false),
                    Arrival = table.Column<string>(type: "character varying(4)", maxLength: 4, nullable: false),
                    Position = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_RunwayConfigs", x => new { x.Session, x.Airport });
                });

            migrationBuilder.CreateTable(
                name: "Strips",
                columns: table => new
                {
                    Session = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Airport = table.Column<string>(type: "character varying(4)", maxLength: 4, nullable: false),
                    Callsign = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    Origin = table.Column<string>(type: "character varying(4)", maxLength: 4, nullable: false),
                    Destination = table.Column<string>(type: "character varying(4)", maxLength: 4, nullable: false),
                    Alternate = table.Column<string>(type: "character varying(4)", maxLength: 4, nullable: false),
                    Route = table.Column<string>(type: "text", nullable: false),
                    Remarks = table.Column<string>(type: "text", nullable: false),
                    AssignedSquawk = table.Column<string>(type: "text", nullable: false),
                    Squawk = table.Column<string>(type: "text", nullable: false),
                    Sid = table.Column<string>(type: "text", nullable: false),
                    ClearedAltitude = table.Column<int>(type: "integer", nullable: false),
                    Heading = table.Column<int>(type: "integer", nullable: true),
                    AircraftType = table.Column<string>(type: "text", nullable: false),
                    Runway = table.Column<string>(type: "text", nullable: false),
                    FinalAltitude = table.Column<int>(type: "integer", nullable: false),
                    Capabilities = table.Column<string>(type: "text", nullable: false),
                    CommunicationType = table.Column<int>(type: "integer", nullable: false),
                    AircraftCategory = table.Column<string>(type: "text", nullable: false),
                    Stand = table.Column<string>(type: "text", nullable: false),
                    Sequence = table.Column<int>(type: "integer", nullable: true),
                    State = table.Column<int>(type: "integer", nullable: false),
                    Cleared = table.Column<bool>(type: "boolean", nullable: false),
                    PositionFrequency = table.Column<string>(type: "character varying(7)", maxLength: 7, nullable: true),
                    BayName = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false),
                    xmin = table.Column<uint>(type: "xid", rowVersion: true, nullable: false),
                    TOBT = table.Column<string>(type: "text", nullable: false),
                    TSAT = table.Column<string>(type: "text", nullable: true),
                    TTOT = table.Column<string>(type: "text", nullable: true),
                    CTOT = table.Column<string>(type: "text", nullable: true),
                    AOBT = table.Column<string>(type: "text", nullable: true),
                    ASAT = table.Column<string>(type: "text", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_Strips", x => new { x.Callsign, x.Session, x.Airport });
                });

            migrationBuilder.CreateTable(
                name: "Coordination",
                columns: table => new
                {
                    Id = table.Column<int>(type: "integer", nullable: false)
                        .Annotation("Npgsql:ValueGenerationStrategy", NpgsqlValueGenerationStrategy.IdentityByDefaultColumn),
                    State = table.Column<int>(type: "integer", nullable: false),
                    Callsign = table.Column<string>(type: "character varying(7)", maxLength: 7, nullable: false),
                    FromFrequency = table.Column<string>(type: "character varying(7)", maxLength: 7, nullable: false),
                    ToFrequency = table.Column<string>(type: "character varying(7)", maxLength: 7, nullable: false),
                    Airport = table.Column<string>(type: "character varying(4)", maxLength: 4, nullable: false),
                    Session = table.Column<string>(type: "character varying(32)", maxLength: 32, nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_Coordination", x => x.Id);
                    table.ForeignKey(
                        name: "FK_Coordination_Strips_Callsign_Session_Airport",
                        columns: x => new { x.Callsign, x.Session, x.Airport },
                        principalTable: "Strips",
                        principalColumns: new[] { "Callsign", "Session", "Airport" },
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateIndex(
                name: "IX_Coordination_Callsign_Session_Airport",
                table: "Coordination",
                columns: new[] { "Callsign", "Session", "Airport" });
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "Coordination");

            migrationBuilder.DropTable(
                name: "OnlinePositions");

            migrationBuilder.DropTable(
                name: "RunwayConfigs");

            migrationBuilder.DropTable(
                name: "Strips");
        }
    }
}
