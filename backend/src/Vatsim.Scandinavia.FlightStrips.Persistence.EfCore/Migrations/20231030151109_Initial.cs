using System;
using Microsoft.EntityFrameworkCore.Metadata;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Migrations
{
    /// <inheritdoc />
    public partial class Initial : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AlterDatabase()
                .Annotation("MySql:CharSet", "utf8mb4");

            migrationBuilder.CreateTable(
                name: "Bays",
                columns: table => new
                {
                    Airport = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Name = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Default = table.Column<bool>(type: "tinyint(1)", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_Bays", x => new { x.Name, x.Airport });
                })
                .Annotation("MySql:CharSet", "utf8mb4");

            migrationBuilder.CreateTable(
                name: "Positions",
                columns: table => new
                {
                    Frequency = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Airport = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Name = table.Column<string>(type: "longtext", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4")
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_Positions", x => new { x.Airport, x.Frequency });
                })
                .Annotation("MySql:CharSet", "utf8mb4");

            migrationBuilder.CreateTable(
                name: "BayFilter",
                columns: table => new
                {
                    Callsign = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    BayEntityAirport = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    BayEntityName = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4")
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_BayFilter", x => x.Callsign);
                    table.ForeignKey(
                        name: "FK_BayFilter_Bays_BayEntityName_BayEntityAirport",
                        columns: x => new { x.BayEntityName, x.BayEntityAirport },
                        principalTable: "Bays",
                        principalColumns: new[] { "Name", "Airport" },
                        onDelete: ReferentialAction.Cascade);
                })
                .Annotation("MySql:CharSet", "utf8mb4");

            migrationBuilder.CreateTable(
                name: "OnlinePositions",
                columns: table => new
                {
                    Session = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Airport = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    PositionName = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    PositionFrequency = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    UpdatedTime = table.Column<DateTime>(type: "datetime(6)", nullable: false)
                        .Annotation("MySql:ValueGenerationStrategy", MySqlValueGenerationStrategy.ComputedColumn)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_OnlinePositions", x => new { x.PositionName, x.Session, x.Airport });
                    table.ForeignKey(
                        name: "FK_OnlinePositions_Positions_PositionFrequency_Airport",
                        columns: x => new { x.PositionFrequency, x.Airport },
                        principalTable: "Positions",
                        principalColumns: new[] { "Airport", "Frequency" },
                        onDelete: ReferentialAction.Cascade);
                })
                .Annotation("MySql:CharSet", "utf8mb4");

            migrationBuilder.CreateTable(
                name: "Strips",
                columns: table => new
                {
                    Session = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Airport = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Callsign = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Origin = table.Column<string>(type: "longtext", nullable: true)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Destination = table.Column<string>(type: "longtext", nullable: true)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Sequence = table.Column<int>(type: "int", nullable: true),
                    State = table.Column<int>(type: "int", nullable: false),
                    Cleared = table.Column<bool>(type: "tinyint(1)", nullable: false),
                    PositionFrequency = table.Column<string>(type: "varchar(255)", nullable: true)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    BayName = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    UpdatedTime = table.Column<DateTime>(type: "datetime(6)", nullable: false)
                        .Annotation("MySql:ValueGenerationStrategy", MySqlValueGenerationStrategy.ComputedColumn)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_Strips", x => new { x.Callsign, x.Session, x.Airport });
                    table.ForeignKey(
                        name: "FK_Strips_Bays_BayName_Airport",
                        columns: x => new { x.BayName, x.Airport },
                        principalTable: "Bays",
                        principalColumns: new[] { "Name", "Airport" },
                        onDelete: ReferentialAction.Cascade);
                    table.ForeignKey(
                        name: "FK_Strips_Positions_PositionFrequency_Airport",
                        columns: x => new { x.PositionFrequency, x.Airport },
                        principalTable: "Positions",
                        principalColumns: new[] { "Airport", "Frequency" });
                })
                .Annotation("MySql:CharSet", "utf8mb4");

            migrationBuilder.CreateTable(
                name: "Coordination",
                columns: table => new
                {
                    Id = table.Column<int>(type: "int", nullable: false)
                        .Annotation("MySql:ValueGenerationStrategy", MySqlValueGenerationStrategy.IdentityColumn),
                    State = table.Column<int>(type: "int", nullable: false),
                    Callsign = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    FromFrequency = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    ToFrequency = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Airport = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4"),
                    Session = table.Column<string>(type: "varchar(255)", nullable: false)
                        .Annotation("MySql:CharSet", "utf8mb4")
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_Coordination", x => x.Id);
                    table.ForeignKey(
                        name: "FK_Coordination_Positions_FromFrequency_Airport",
                        columns: x => new { x.FromFrequency, x.Airport },
                        principalTable: "Positions",
                        principalColumns: new[] { "Airport", "Frequency" },
                        onDelete: ReferentialAction.Cascade);
                    table.ForeignKey(
                        name: "FK_Coordination_Positions_ToFrequency_Airport",
                        columns: x => new { x.ToFrequency, x.Airport },
                        principalTable: "Positions",
                        principalColumns: new[] { "Airport", "Frequency" },
                        onDelete: ReferentialAction.Cascade);
                    table.ForeignKey(
                        name: "FK_Coordination_Strips_Callsign_Airport_Session",
                        columns: x => new { x.Callsign, x.Airport, x.Session },
                        principalTable: "Strips",
                        principalColumns: new[] { "Callsign", "Session", "Airport" },
                        onDelete: ReferentialAction.Cascade);
                })
                .Annotation("MySql:CharSet", "utf8mb4");

            migrationBuilder.CreateIndex(
                name: "IX_BayFilter_BayEntityName_BayEntityAirport",
                table: "BayFilter",
                columns: new[] { "BayEntityName", "BayEntityAirport" });

            migrationBuilder.CreateIndex(
                name: "IX_Coordination_Callsign_Airport_Session",
                table: "Coordination",
                columns: new[] { "Callsign", "Airport", "Session" });

            migrationBuilder.CreateIndex(
                name: "IX_Coordination_FromFrequency_Airport",
                table: "Coordination",
                columns: new[] { "FromFrequency", "Airport" });

            migrationBuilder.CreateIndex(
                name: "IX_Coordination_ToFrequency_Airport",
                table: "Coordination",
                columns: new[] { "ToFrequency", "Airport" });

            migrationBuilder.CreateIndex(
                name: "IX_OnlinePositions_PositionFrequency_Airport",
                table: "OnlinePositions",
                columns: new[] { "PositionFrequency", "Airport" });

            migrationBuilder.CreateIndex(
                name: "IX_Strips_BayName_Airport",
                table: "Strips",
                columns: new[] { "BayName", "Airport" });

            migrationBuilder.CreateIndex(
                name: "IX_Strips_PositionFrequency_Airport",
                table: "Strips",
                columns: new[] { "PositionFrequency", "Airport" });
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "BayFilter");

            migrationBuilder.DropTable(
                name: "Coordination");

            migrationBuilder.DropTable(
                name: "OnlinePositions");

            migrationBuilder.DropTable(
                name: "Strips");

            migrationBuilder.DropTable(
                name: "Bays");

            migrationBuilder.DropTable(
                name: "Positions");
        }
    }
}
